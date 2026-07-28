// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

type UserResource struct {
	client *proxmoxBackupServerClient
}

type UserResourceModel struct {
	UserID    types.String `tfsdk:"user_id"`
	Enable    types.Bool   `tfsdk:"enable"`
	Comment   types.String `tfsdk:"comment"`
	Email     types.String `tfsdk:"email"`
	Firstname types.String `tfsdk:"first_name"`
	Lastname  types.String `tfsdk:"last_name"`
	Expire    types.Int64  `tfsdk:"expire"`
}

func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_user"
}

func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server user via `/access/users`.",
		Attributes: map[string]schema.Attribute{
			"user_id": schema.StringAttribute{
				MarkdownDescription: "Proxmox Backup Server user ID, for example `homepage@pbs`.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enable": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is enabled.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"comment": schema.StringAttribute{
				MarkdownDescription: "User comment.",
				Optional:            true,
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "User email address.",
				Optional:            true,
			},
			"first_name": schema.StringAttribute{
				MarkdownDescription: "User first name.",
				Optional:            true,
			},
			"last_name": schema.StringAttribute{
				MarkdownDescription: "User last name.",
				Optional:            true,
			},
			"expire": schema.Int64Attribute{
				MarkdownDescription: "Account expiration time as epoch seconds. A value of `0` means no expiration.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
			},
		},
	}
}

func (r *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.withConfigMutation(ctx, r.readUsersDigest, func(ctx context.Context, _ *string) error {
		return r.client.post(ctx, "/access/users", userPayload(data), nil)
	}); err != nil {
		resp.Diagnostics.AddError("Create User Failed", err.Error())
		return
	}
	recoveryState := userRecoveryState(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &recoveryState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readUser(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read User Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readUser(ctx, &data); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read User Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan UserResourceModel
	var state UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleted := userDeletedFields(plan, state)
	if err := r.client.withConfigMutation(ctx, func(ctx context.Context) (*string, error) {
		return r.readUserDigest(ctx, plan.UserID.ValueString())
	}, func(ctx context.Context, digest *string) error {
		payload := userPayload(plan)
		payload.Delete = deleted
		payload.Digest = digest
		return r.client.put(ctx, "/access/users/"+urlPathEscape(plan.UserID.ValueString()), payload)
	}); err != nil {
		resp.Diagnostics.AddError("Update User Failed", err.Error())
		return
	}
	recoveryState := userRecoveryState(plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &recoveryState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readUser(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read User Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.withConfigMutation(ctx, func(ctx context.Context) (*string, error) {
		return r.readUserDigest(ctx, data.UserID.ValueString())
	}, func(ctx context.Context, digest *string) error {
		form := url.Values{}
		setAccessDigest(form, digest)
		return r.client.deleteForm(ctx, "/access/users/"+urlPathEscape(data.UserID.ValueString()), form)
	})
	if err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			return
		}
		resp.Diagnostics.AddError("Delete User Failed", err.Error())
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("user_id"), req, resp)
}

func (r *UserResource) readUser(ctx context.Context, data *UserResourceModel) error {
	var apiData userAPIModel
	if err := r.client.get(ctx, "/access/users/"+urlPathEscape(data.UserID.ValueString()), &apiData); err != nil {
		return err
	}
	data.UserID = types.StringValue(apiData.UserID)
	data.Enable = accessBoolPointerValue(apiData.Enable)
	data.Comment = accessStringValue(apiData.Comment)
	data.Email = accessStringValue(apiData.Email)
	data.Firstname = accessStringValue(apiData.Firstname)
	data.Lastname = accessStringValue(apiData.Lastname)
	data.Expire = accessInt64Value(apiData.Expire)
	return nil
}

func userPayload(data UserResourceModel) userAPIModel {
	return userAPIModel{
		UserID:    data.UserID.ValueString(),
		Enable:    accessBoolPointer(data.Enable),
		Comment:   accessStringPointer(data.Comment),
		Email:     accessStringPointer(data.Email),
		Firstname: accessStringPointer(data.Firstname),
		Lastname:  accessStringPointer(data.Lastname),
		Expire:    accessInt64Pointer(data.Expire),
	}
}

func (r *UserResource) readUsersDigest(ctx context.Context) (*string, error) {
	var users []userAPIModel
	return r.client.getWithDigest(ctx, "/access/users", &users)
}

func (r *UserResource) readUserDigest(ctx context.Context, userID string) (*string, error) {
	var apiData userAPIModel
	return r.client.getWithDigest(ctx, "/access/users/"+urlPathEscape(userID), &apiData)
}

func userDeletedFields(plan, state UserResourceModel) []string {
	var deleted []string
	for _, field := range []struct {
		name  string
		plan  types.String
		state types.String
	}{
		{name: "comment", plan: plan.Comment, state: state.Comment},
		{name: "email", plan: plan.Email, state: state.Email},
		{name: "firstname", plan: plan.Firstname, state: state.Firstname},
		{name: "lastname", plan: plan.Lastname, state: state.Lastname},
	} {
		if field.plan.IsNull() && !field.state.IsNull() && !field.state.IsUnknown() {
			deleted = append(deleted, field.name)
		}
	}
	return deleted
}

func userRecoveryState(data UserResourceModel) UserResourceModel {
	recovery := data
	if recovery.Enable.IsUnknown() {
		recovery.Enable = types.BoolNull()
	}
	if recovery.Comment.IsUnknown() {
		recovery.Comment = types.StringNull()
	}
	if recovery.Email.IsUnknown() {
		recovery.Email = types.StringNull()
	}
	if recovery.Firstname.IsUnknown() {
		recovery.Firstname = types.StringNull()
	}
	if recovery.Lastname.IsUnknown() {
		recovery.Lastname = types.StringNull()
	}
	if recovery.Expire.IsUnknown() {
		recovery.Expire = types.Int64Null()
	}
	return recovery
}

func setAccessDigest(form url.Values, digest *string) {
	if digest != nil && *digest != "" {
		form.Set("digest", *digest)
	}
}
