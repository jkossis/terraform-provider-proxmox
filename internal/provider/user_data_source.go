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

var _ datasource.DataSource = &UserDataSource{}

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

type UserDataSource struct {
	client *proxmoxBackupServerClient
}

type UserDataSourceModel struct {
	UserID    types.String `tfsdk:"user_id"`
	Enable    types.Bool   `tfsdk:"enable"`
	Comment   types.String `tfsdk:"comment"`
	Email     types.String `tfsdk:"email"`
	Firstname types.String `tfsdk:"first_name"`
	Lastname  types.String `tfsdk:"last_name"`
	Expire    types.Int64  `tfsdk:"expire"`
}

func (d *UserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_user"
}

func (d *UserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Proxmox Backup Server user via `/access/users/{user_id}`.",
		Attributes: map[string]schema.Attribute{
			"user_id":    schema.StringAttribute{MarkdownDescription: "Proxmox Backup Server user ID.", Required: true},
			"enable":     schema.BoolAttribute{MarkdownDescription: "Whether the user is enabled.", Computed: true},
			"comment":    schema.StringAttribute{MarkdownDescription: "User comment.", Computed: true},
			"email":      schema.StringAttribute{MarkdownDescription: "User email address.", Computed: true},
			"first_name": schema.StringAttribute{MarkdownDescription: "User first name.", Computed: true},
			"last_name":  schema.StringAttribute{MarkdownDescription: "User last name.", Computed: true},
			"expire":     schema.Int64Attribute{MarkdownDescription: "Account expiration time as epoch seconds. A value of `0` means no expiration.", Computed: true},
		},
	}
}

func (d *UserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data UserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiData userAPIModel
	if err := d.client.get(ctx, "/access/users/"+urlPathEscape(data.UserID.ValueString()), &apiData); err != nil {
		resp.Diagnostics.AddError("Read User Failed", err.Error())
		return
	}
	data.UserID = types.StringValue(apiData.UserID)
	data.Enable = accessBoolPointerValue(apiData.Enable)
	data.Comment = accessStringValue(apiData.Comment)
	data.Email = accessStringValue(apiData.Email)
	data.Firstname = accessStringValue(apiData.Firstname)
	data.Lastname = accessStringValue(apiData.Lastname)
	data.Expire = accessInt64Value(apiData.Expire)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
