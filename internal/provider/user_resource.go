// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
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
	UserID  types.String `tfsdk:"userid"`
	Enable  types.Bool   `tfsdk:"enable"`
	Comment types.String `tfsdk:"comment"`
}

func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_user"
}

func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server user via `/access/users`.",
		Attributes: map[string]schema.Attribute{
			"userid": schema.StringAttribute{
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

	if err := r.client.post(ctx, "/access/users", userPayload(data), nil); err != nil {
		resp.Diagnostics.AddError("Create User Failed", err.Error())
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
	var data UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.put(ctx, "/access/users/"+urlPathEscape(data.UserID.ValueString()), userPayload(data)); err != nil {
		resp.Diagnostics.AddError("Update User Failed", err.Error())
		return
	}
	if err := r.readUser(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read User Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.delete(ctx, "/access/users/"+urlPathEscape(data.UserID.ValueString())); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			return
		}
		resp.Diagnostics.AddError("Delete User Failed", err.Error())
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("userid"), req, resp)
}

func (r *UserResource) readUser(ctx context.Context, data *UserResourceModel) error {
	var apiData userAPIModel
	if err := r.client.get(ctx, "/access/users/"+urlPathEscape(data.UserID.ValueString()), &apiData); err != nil {
		return err
	}
	data.UserID = types.StringValue(apiData.UserID)
	data.Enable = accessBoolPointerValue(apiData.Enable)
	if apiData.Comment == "" {
		data.Comment = types.StringNull()
	} else {
		data.Comment = types.StringValue(apiData.Comment)
	}
	return nil
}

func userPayload(data UserResourceModel) userAPIModel {
	return userAPIModel{UserID: data.UserID.ValueString(), Enable: accessBoolPointer(data.Enable), Comment: data.Comment.ValueString()}
}
