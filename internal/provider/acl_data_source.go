// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ACLDataSource{}

func NewACLDataSource() datasource.DataSource { return &ACLDataSource{} }

type ACLDataSource struct{ client *proxmoxBackupServerClient }

type ACLDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Path      types.String `tfsdk:"path"`
	AuthID    types.String `tfsdk:"auth_id"`
	Role      types.String `tfsdk:"role"`
	Propagate types.Bool   `tfsdk:"propagate"`
}

func (d *ACLDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_acl"
}

func (d *ACLDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Proxmox Backup Server ACL entry via `/access/acl`.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{MarkdownDescription: "ACL entry ID in `path|auth_id|role` format.", Computed: true},
			"path":      schema.StringAttribute{MarkdownDescription: "ACL path.", Required: true},
			"auth_id":   schema.StringAttribute{MarkdownDescription: "User or token auth ID for this ACL entry.", Required: true},
			"role":      schema.StringAttribute{MarkdownDescription: "Role assigned by this ACL entry.", Required: true},
			"propagate": schema.BoolAttribute{MarkdownDescription: "Whether this ACL entry propagates to child paths.", Computed: true},
		},
	}
}

func (d *ACLDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*proxmoxBackupServerClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected *proxmoxBackupServerClient, got: %T. Please report this issue to the provider developers.", req.ProviderData))
		return
	}
	d.client = client
}

func (d *ACLDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ACLDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var entries []aclAPIModel
	if err := d.client.get(ctx, "/access/acl", &entries); err != nil {
		resp.Diagnostics.AddError("Read ACL Failed", err.Error())
		return
	}
	for _, entry := range entries {
		if entry.Path == data.Path.ValueString() && aclAPIAuthID(entry) == data.AuthID.ValueString() && aclAPIRole(entry) == data.Role.ValueString() {
			data.ID = types.StringValue(aclEntryID(entry.Path, aclAPIAuthID(entry), aclAPIRole(entry)))
			data.Path = types.StringValue(entry.Path)
			data.AuthID = types.StringValue(aclAPIAuthID(entry))
			data.Role = types.StringValue(aclAPIRole(entry))
			data.Propagate = accessBoolPointerValue(entry.Propagate)
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			return
		}
	}
	resp.Diagnostics.AddError("Read ACL Failed", "ACL entry not found")
}
