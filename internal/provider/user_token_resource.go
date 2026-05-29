// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &UserTokenResource{}
var _ resource.ResourceWithImportState = &UserTokenResource{}

func NewUserTokenResource() resource.Resource { return &UserTokenResource{} }

type UserTokenResource struct{ client *proxmoxBackupServerClient }

type UserTokenResourceModel struct {
	ID        types.String `tfsdk:"id"`
	UserID    types.String `tfsdk:"userid"`
	TokenName types.String `tfsdk:"token_name"`
	Enable    types.Bool   `tfsdk:"enable"`
	Comment   types.String `tfsdk:"comment"`
	Value     types.String `tfsdk:"value"`
}

func (r *UserTokenResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_user_token"
}

func (r *UserTokenResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server user API token via `/access/users/{userid}/token/{token_name}`.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{MarkdownDescription: "Token auth ID in `userid!token_name` format.", Computed: true},
			"userid":     schema.StringAttribute{MarkdownDescription: "Proxmox Backup Server user ID that owns the token.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"token_name": schema.StringAttribute{MarkdownDescription: "Token name.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"enable":     schema.BoolAttribute{MarkdownDescription: "Whether the token is enabled.", Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"comment":    schema.StringAttribute{MarkdownDescription: "Token comment.", Optional: true},
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
	if err := r.client.post(ctx, userTokenPath(data.UserID.ValueString(), data.TokenName.ValueString()), userTokenPayload(data), &createResp); err != nil {
		resp.Diagnostics.AddError("Create User Token Failed", err.Error())
		return
	}
	if createResp.Value != "" {
		data.Value = types.StringValue(createResp.Value)
	}
	if err := r.readUserToken(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read User Token Failed", err.Error())
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
		resp.Diagnostics.AddError("Read User Token Failed", err.Error())
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
	if err := r.client.put(ctx, userTokenPath(plan.UserID.ValueString(), plan.TokenName.ValueString()), userTokenPayload(plan)); err != nil {
		resp.Diagnostics.AddError("Update User Token Failed", err.Error())
		return
	}
	if err := r.readUserToken(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read User Token Failed", err.Error())
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
	if err := r.client.delete(ctx, userTokenPath(data.UserID.ValueString(), data.TokenName.ValueString())); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			return
		}
		resp.Diagnostics.AddError("Delete User Token Failed", err.Error())
	}
}

func (r *UserTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	userid, tokenName, ok := strings.Cut(req.ID, "!")
	if !ok || userid == "" || tokenName == "" {
		resp.Diagnostics.AddError("Invalid User Token Import ID", "Expected import ID in `userid!token_name` format.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("userid"), userid)...)
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
	if apiData.Comment == "" {
		data.Comment = types.StringNull()
	} else {
		data.Comment = types.StringValue(apiData.Comment)
	}
	data.Value = configuredValue
	return nil
}

func userTokenPayload(data UserTokenResourceModel) userTokenAPIModel {
	return userTokenAPIModel{Enable: accessBoolPointer(data.Enable), Comment: data.Comment.ValueString()}
}

func userTokenPath(userid, tokenName string) string {
	return "/access/users/" + urlPathEscape(userid) + "/token/" + urlPathEscape(tokenName)
}
