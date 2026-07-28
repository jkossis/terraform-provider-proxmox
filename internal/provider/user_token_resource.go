// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &UserTokenResource{}
var _ resource.ResourceWithImportState = &UserTokenResource{}

func NewUserTokenResource() resource.Resource { return &UserTokenResource{} }

type UserTokenResource struct{ client *proxmoxBackupServerClient }

type UserTokenResourceModel struct {
	ID         types.String `tfsdk:"id"`
	UserID     types.String `tfsdk:"user_id"`
	TokenName  types.String `tfsdk:"token_name"`
	Enable     types.Bool   `tfsdk:"enable"`
	Comment    types.String `tfsdk:"comment"`
	Expire     types.Int64  `tfsdk:"expire"`
	Regenerate types.Bool   `tfsdk:"regenerate"`
	Value      types.String `tfsdk:"value"`
}

func (r *UserTokenResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_user_token"
}

func (r *UserTokenResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server user API token via `/access/users/{user_id}/token/{token_name}`.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{MarkdownDescription: "Token auth ID in `user_id!token_name` format.", Computed: true},
			"user_id":    schema.StringAttribute{MarkdownDescription: "Proxmox Backup Server user ID that owns the token.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"token_name": schema.StringAttribute{MarkdownDescription: "Token name.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable":     schema.BoolAttribute{MarkdownDescription: "Whether the token is enabled.", Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"comment":    schema.StringAttribute{MarkdownDescription: "Token comment.", Optional: true},
			"expire":     schema.Int64Attribute{MarkdownDescription: "Token expiration time as epoch seconds. A value of `0` means no expiration.", Optional: true, Computed: true, Default: int64default.StaticInt64(0)},
			"regenerate": schema.BoolAttribute{MarkdownDescription: "Regenerate the token secret on a false-to-true transition. Set this to `false` and apply before setting it to `true` again.", Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"value":      schema.StringAttribute{MarkdownDescription: "Token secret value. Proxmox Backup Server only returns this during token creation.", Computed: true, Sensitive: true},
		},
	}
}

func (r *UserTokenResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*proxmoxBackupServerClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *proxmoxBackupServerClient, got: %T. Please report this issue to the provider developers.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *UserTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var createResp userTokenAPIModel
	if err := r.client.withConfigMutation(ctx, func(ctx context.Context) (*string, error) {
		return r.readUserTokenListDigest(ctx, data.UserID.ValueString())
	}, func(ctx context.Context, digest *string) error {
		payload := userTokenPayload(data)
		payload.Digest = digest
		return r.client.post(ctx, userTokenPath(data.UserID.ValueString(), data.TokenName.ValueString()), payload, &createResp)
	}); err != nil {
		resp.Diagnostics.AddError("Create User Token Failed", userTokenError(err, data.Value))
		return
	}
	if secret := userTokenSecret(createResp); secret != "" {
		data.Value = types.StringValue(secret)
	}
	recoveryState := userTokenRecoveryState(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &recoveryState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readUserToken(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read User Token Failed", userTokenError(err, data.Value))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readUserToken(ctx, &data); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read User Token Failed", userTokenError(err, data.Value))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan UserTokenResourceModel
	var state UserTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Value = state.Value
	regenerate := userTokenRegenerateEdge(plan.Regenerate, state.Regenerate)
	var mutationResp userTokenAPIModel
	if err := r.client.withConfigMutation(ctx, func(ctx context.Context) (*string, error) {
		return r.readUserTokenDigest(ctx, plan.UserID.ValueString(), plan.TokenName.ValueString())
	}, func(ctx context.Context, digest *string) error {
		return r.client.putWithResponse(ctx, userTokenPath(plan.UserID.ValueString(), plan.TokenName.ValueString()), userTokenMutationPayload(plan, digest, regenerate), &mutationResp)
	}); err != nil {
		resp.Diagnostics.AddError("Update User Token Failed", userTokenError(err, state.Value, plan.Value))
		return
	}
	if secret := userTokenSecret(mutationResp); secret != "" {
		plan.Value = types.StringValue(secret)
	}
	recoveryState := userTokenRecoveryState(plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &recoveryState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readUserToken(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read User Token Failed", userTokenError(err, state.Value, plan.Value))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.withConfigMutation(ctx, func(ctx context.Context) (*string, error) {
		return r.readUserTokenDigest(ctx, data.UserID.ValueString(), data.TokenName.ValueString())
	}, func(ctx context.Context, digest *string) error {
		form := url.Values{}
		setAccessDigest(form, digest)
		return r.client.deleteForm(ctx, userTokenPath(data.UserID.ValueString(), data.TokenName.ValueString()), form)
	})
	if err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			return
		}
		resp.Diagnostics.AddError("Delete User Token Failed", userTokenError(err, data.Value))
	}
}

func (r *UserTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	userID, tokenName, ok := strings.Cut(req.ID, "!")
	if !ok || userID == "" || tokenName == "" {
		resp.Diagnostics.AddError("Invalid User Token Import ID", "Expected import ID in `user_id!token_name` format.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), userID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("token_name"), tokenName)...)
}

func (r *UserTokenResource) readUserToken(ctx context.Context, data *UserTokenResourceModel) error {
	configuredValue := data.Value
	var apiData userTokenAPIModel
	if err := r.client.get(ctx, userTokenPath(data.UserID.ValueString(), data.TokenName.ValueString()), &apiData); err != nil {
		return err
	}
	data.ID = types.StringValue(data.UserID.ValueString() + "!" + data.TokenName.ValueString())
	data.Enable = accessBoolPointerValue(apiData.Enable)
	data.Comment = accessStringValue(apiData.Comment)
	data.Expire = accessInt64Value(apiData.Expire)
	if data.Regenerate.IsNull() || data.Regenerate.IsUnknown() {
		data.Regenerate = types.BoolValue(false)
	}
	data.Value = configuredValue
	return nil
}

func userTokenPayload(data UserTokenResourceModel) userTokenAPIModel {
	return userTokenAPIModel{Enable: accessBoolPointer(data.Enable), Comment: accessStringPointer(data.Comment), Expire: accessInt64Pointer(data.Expire)}
}

func userTokenMutationPayload(data UserTokenResourceModel, digest *string, regenerate bool) userTokenAPIModel {
	payload := userTokenPayload(data)
	payload.Digest = digest
	if data.Comment.IsNull() && !data.Comment.IsUnknown() {
		payload.Delete = []string{"comment"}
	}
	if regenerate {
		value := proxmoxBackupServerBool(true)
		payload.Regenerate = &value
	}
	return payload
}

func userTokenPath(userID, tokenName string) string {
	return "/access/users/" + urlPathEscape(userID) + "/token/" + urlPathEscape(tokenName)
}

func (r *UserTokenResource) readUserTokenListDigest(ctx context.Context, userID string) (*string, error) {
	var tokens []userTokenAPIModel
	return r.client.getWithDigest(ctx, "/access/users/"+urlPathEscape(userID)+"/token", &tokens)
}

func (r *UserTokenResource) readUserTokenDigest(ctx context.Context, userID, tokenName string) (*string, error) {
	var apiData userTokenAPIModel
	return r.client.getWithDigest(ctx, userTokenPath(userID, tokenName), &apiData)
}

func userTokenRegenerateEdge(plan, state types.Bool) bool {
	if plan.IsNull() || plan.IsUnknown() || !plan.ValueBool() {
		return false
	}
	return state.IsNull() || state.IsUnknown() || !state.ValueBool()
}

func userTokenSecret(data userTokenAPIModel) string {
	if data.Secret != "" {
		return data.Secret
	}
	return data.Value
}

func userTokenRecoveryState(data UserTokenResourceModel) UserTokenResourceModel {
	recovery := data
	recovery.ID = types.StringValue(data.UserID.ValueString() + "!" + data.TokenName.ValueString())
	if recovery.Enable.IsUnknown() {
		recovery.Enable = types.BoolNull()
	}
	if recovery.Comment.IsUnknown() {
		recovery.Comment = types.StringNull()
	}
	if recovery.Expire.IsUnknown() {
		recovery.Expire = types.Int64Null()
	}
	if recovery.Regenerate.IsUnknown() {
		recovery.Regenerate = types.BoolNull()
	}
	if recovery.Value.IsUnknown() {
		recovery.Value = types.StringNull()
	}
	return recovery
}

func userTokenError(err error, secrets ...types.String) string {
	message := err.Error()
	var apiErr *proxmoxBackupServerAPIError
	if errors.As(err, &apiErr) {
		// API error bodies can contain a newly generated secret that is not
		// available to the caller when response decoding fails. Keep the
		// diagnostic useful without ever echoing that body.
		message = fmt.Sprintf("%s %s failed: %s", apiErr.method, apiErr.path, apiErr.status)
	}
	for _, secretValue := range secrets {
		if secretValue.IsNull() || secretValue.IsUnknown() || secretValue.ValueString() == "" {
			continue
		}
		secret := secretValue.ValueString()
		variants := []string{secret, url.QueryEscape(secret), url.PathEscape(secret), strconv.Quote(secret)}
		if encoded, marshalErr := json.Marshal(secret); marshalErr == nil {
			variants = append(variants, string(encoded))
		}
		for _, variant := range variants {
			message = strings.ReplaceAll(message, variant, "[REDACTED]")
		}
	}
	return message
}
