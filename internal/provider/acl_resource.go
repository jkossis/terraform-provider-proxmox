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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ACLResource{}
var _ resource.ResourceWithImportState = &ACLResource{}

func NewACLResource() resource.Resource { return &ACLResource{} }

type ACLResource struct{ client *proxmoxBackupServerClient }

type ACLResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Path      types.String `tfsdk:"path"`
	AuthID    types.String `tfsdk:"user_id"`
	UGIDType  types.String `tfsdk:"ugid_type"`
	Role      types.String `tfsdk:"role_id"`
	Propagate types.Bool   `tfsdk:"propagate"`
}

func (r *ACLResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_acl"
}

func (r *ACLResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server ACL entry via `/access/acl`.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{MarkdownDescription: "ACL entry ID in `path|user_id|role_id` format for users and tokens, or `path|group|user_id|role_id` format for groups.", Computed: true},
			"path":      schema.StringAttribute{MarkdownDescription: "ACL path, for example `/`.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"user_id":   schema.StringAttribute{MarkdownDescription: "User, token, or group ID for this ACL entry.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"ugid_type": schema.StringAttribute{MarkdownDescription: "ACL subject type: `user` or `group`. Defaults to `user`; use `group` for a group entry.", Optional: true, Computed: true, Default: stringdefault.StaticString("user"), Validators: []validator.String{aclUGIDTypeValidator{}}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"role_id":   schema.StringAttribute{MarkdownDescription: "Role assigned by this ACL entry, for example `Audit`.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
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
	if err := r.client.withConfigMutation(ctx, r.readACLDigest, func(ctx context.Context, digest *string) error {
		return r.client.put(ctx, "/access/acl", aclPayloadWithDigest(data, false, digest))
	}); err != nil {
		resp.Diagnostics.AddError("Create ACL Failed", err.Error())
		return
	}
	recoveryState := aclRecoveryState(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &recoveryState)...)
	if resp.Diagnostics.HasError() {
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
	if err := r.client.withConfigMutation(ctx, r.readACLDigest, func(ctx context.Context, digest *string) error {
		return r.client.put(ctx, "/access/acl", aclPayloadWithDigest(data, false, digest))
	}); err != nil {
		resp.Diagnostics.AddError("Update ACL Failed", err.Error())
		return
	}
	recoveryState := aclRecoveryState(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &recoveryState)...)
	if resp.Diagnostics.HasError() {
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
	err := r.client.withConfigMutation(ctx, r.readACLDigest, func(ctx context.Context, digest *string) error {
		return r.client.put(ctx, "/access/acl", aclPayloadWithDigest(data, true, digest))
	})
	if err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			return
		}
		resp.Diagnostics.AddError("Delete ACL Failed", err.Error())
	}
}

func (r *ACLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "|")
	if len(parts) == 3 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[1])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), parts[2])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ugid_type"), "user")...)
		return
	}
	if len(parts) == 4 {
		// The canonical extended form is path|ugid_type|user_id|role_id. Accept
		// the additive path|user_id|role_id|ugid_type spelling as well.
		ugidType, authID, role := parts[1], parts[2], parts[3]
		if !validACLUGIDType(ugidType) && validACLUGIDType(parts[3]) {
			ugidType, authID, role = parts[3], parts[1], parts[2]
		}
		if validACLUGIDType(ugidType) {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), parts[0])...)
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), authID)...)
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), role)...)
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ugid_type"), ugidType)...)
			return
		}
	}
	resp.Diagnostics.AddError("Invalid ACL Import ID", "Expected import ID in `path|user_id|role_id` format for users or tokens, or `path|group|user_id|role_id` format for groups.")
}

func (r *ACLResource) readACL(ctx context.Context, data *ACLResourceModel) error {
	var entries []aclAPIModel
	if err := r.client.get(ctx, "/access/acl", &entries); err != nil {
		return err
	}
	wantType := aclUGIDTypeValue(data.UGIDType)
	for _, entry := range entries {
		if entry.Path == data.Path.ValueString() && aclAPIUGIDType(entry) == wantType && aclAPIAuthID(entry) == data.AuthID.ValueString() && aclAPIRole(entry) == data.Role.ValueString() {
			data.ID = types.StringValue(aclEntryIDForType(entry.Path, aclAPIAuthID(entry), aclAPIRole(entry), aclAPIUGIDType(entry)))
			data.Path = types.StringValue(entry.Path)
			data.AuthID = types.StringValue(aclAPIAuthID(entry))
			data.UGIDType = types.StringValue(aclAPIUGIDType(entry))
			data.Role = types.StringValue(aclAPIRole(entry))
			data.Propagate = accessBoolPointerValue(entry.Propagate)
			return nil
		}
	}
	return &proxmoxBackupServerAPIError{method: "GET", path: "/api2/json/access/acl", status: "404 Not Found", code: 404, body: "ACL entry not found"}
}

func aclPayload(data ACLResourceModel, deleteEntry bool) aclMutationModel {
	payload := aclMutationModel{Path: data.Path.ValueString(), Role: data.Role.ValueString()}
	if aclUGIDTypeValue(data.UGIDType) == "group" {
		payload.Group = data.AuthID.ValueString()
	} else {
		payload.AuthID = data.AuthID.ValueString()
	}
	if deleteEntry {
		payload.Delete = accessBoolPointer(types.BoolValue(true))
	} else {
		payload.Propagate = accessBoolPointer(data.Propagate)
	}
	return payload
}

func aclPayloadWithDigest(data ACLResourceModel, deleteEntry bool, digest *string) aclMutationModel {
	payload := aclPayload(data, deleteEntry)
	payload.Digest = digest
	return payload
}

func (r *ACLResource) readACLDigest(ctx context.Context) (*string, error) {
	var entries []aclAPIModel
	return r.client.getWithDigest(ctx, "/access/acl", &entries)
}

func aclUGIDTypeValue(value types.String) string {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return "user"
	}
	return value.ValueString()
}

func validACLUGIDType(value string) bool {
	return value == "user" || value == "group"
}

func aclEntryIDForType(path, authID, role, ugidType string) string {
	if ugidType == "group" {
		return path + "|group|" + authID + "|" + role
	}
	return aclEntryID(path, authID, role)
}

func aclRecoveryState(data ACLResourceModel) ACLResourceModel {
	recovery := data
	recovery.UGIDType = types.StringValue(aclUGIDTypeValue(data.UGIDType))
	recovery.ID = types.StringValue(aclEntryIDForType(data.Path.ValueString(), data.AuthID.ValueString(), data.Role.ValueString(), recovery.UGIDType.ValueString()))
	if recovery.Propagate.IsUnknown() {
		recovery.Propagate = types.BoolNull()
	}
	return recovery
}
