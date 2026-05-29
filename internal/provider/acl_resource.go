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

var _ resource.Resource = &ACLResource{}
var _ resource.ResourceWithImportState = &ACLResource{}

func NewACLResource() resource.Resource { return &ACLResource{} }

type ACLResource struct{ client *proxmoxBackupServerClient }

type ACLResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Path      types.String `tfsdk:"path"`
	AuthID    types.String `tfsdk:"auth_id"`
	Role      types.String `tfsdk:"role"`
	Propagate types.Bool   `tfsdk:"propagate"`
}

func (r *ACLResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_acl"
}

func (r *ACLResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server ACL entry via `/access/acl`.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{MarkdownDescription: "ACL entry ID in `path|auth_id|role` format.", Computed: true},
			"path":      schema.StringAttribute{MarkdownDescription: "ACL path, for example `/`.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"auth_id":   schema.StringAttribute{MarkdownDescription: "User or token auth ID for this ACL entry.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"role":      schema.StringAttribute{MarkdownDescription: "Role assigned by this ACL entry, for example `Audit`.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"propagate": schema.BoolAttribute{MarkdownDescription: "Whether this ACL entry propagates to child paths.", Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
		},
	}
}

func (r *ACLResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ACLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ACLResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.put(ctx, "/access/acl", aclPayload(data, false)); err != nil {
		resp.Diagnostics.AddError("Create ACL Failed", err.Error())
		return
	}
	if err := r.readACL(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read ACL Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ACLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ACLResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readACL(ctx, &data); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read ACL Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ACLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ACLResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.put(ctx, "/access/acl", aclPayload(data, false)); err != nil {
		resp.Diagnostics.AddError("Update ACL Failed", err.Error())
		return
	}
	if err := r.readACL(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read ACL Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ACLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ACLResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.put(ctx, "/access/acl", aclPayload(data, true)); err != nil {
		resp.Diagnostics.AddError("Delete ACL Failed", err.Error())
	}
}

func (r *ACLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "|")
	if len(parts) != 3 {
		resp.Diagnostics.AddError("Invalid ACL Import ID", "Expected import ID in `path|auth_id|role` format.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("auth_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role"), parts[2])...)
}

func (r *ACLResource) readACL(ctx context.Context, data *ACLResourceModel) error {
	var entries []aclAPIModel
	if err := r.client.get(ctx, "/access/acl", &entries); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Path == data.Path.ValueString() && aclAPIAuthID(entry) == data.AuthID.ValueString() && aclAPIRole(entry) == data.Role.ValueString() {
			data.ID = types.StringValue(aclEntryID(entry.Path, aclAPIAuthID(entry), aclAPIRole(entry)))
			data.Path = types.StringValue(entry.Path)
			data.AuthID = types.StringValue(aclAPIAuthID(entry))
			data.Role = types.StringValue(aclAPIRole(entry))
			data.Propagate = accessBoolPointerValue(entry.Propagate)
			return nil
		}
	}
	return &proxmoxBackupServerAPIError{method: "GET", path: "/api2/json/access/acl", status: "404 Not Found", code: 404, body: "ACL entry not found"}
}

func aclPayload(data ACLResourceModel, deleteEntry bool) aclAPIModel {
	payload := aclAPIModel{Path: data.Path.ValueString(), AuthID: data.AuthID.ValueString(), Role: data.Role.ValueString()}
	if deleteEntry {
		payload.Delete = accessBoolPointer(types.BoolValue(true))
	} else {
		payload.Propagate = accessBoolPointer(data.Propagate)
	}
	return payload
}
