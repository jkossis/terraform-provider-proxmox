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

var _ datasource.DataSource = &UserTokenDataSource{}

func NewUserTokenDataSource() datasource.DataSource { return &UserTokenDataSource{} }

type UserTokenDataSource struct{ client *proxmoxBackupServerClient }

type UserTokenDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	UserID    types.String `tfsdk:"userid"`
	TokenName types.String `tfsdk:"token_name"`
	Enable    types.Bool   `tfsdk:"enable"`
	Comment   types.String `tfsdk:"comment"`
}

func (d *UserTokenDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_user_token"
}

func (d *UserTokenDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Proxmox Backup Server user API token via `/access/users/{userid}/token/{token_name}`.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{MarkdownDescription: "Token auth ID in `userid!token_name` format.", Computed: true},
			"userid":     schema.StringAttribute{MarkdownDescription: "Proxmox Backup Server user ID that owns the token.", Required: true},
			"token_name": schema.StringAttribute{MarkdownDescription: "Token name.", Required: true},
			"enable":     schema.BoolAttribute{MarkdownDescription: "Whether the token is enabled.", Computed: true},
			"comment":    schema.StringAttribute{MarkdownDescription: "Token comment.", Computed: true},
		},
	}
}

func (d *UserTokenDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UserTokenDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data UserTokenDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiData userTokenAPIModel
	if err := d.client.get(ctx, userTokenPath(data.UserID.ValueString(), data.TokenName.ValueString()), &apiData); err != nil {
		resp.Diagnostics.AddError("Read User Token Failed", err.Error())
		return
	}
	data.ID = types.StringValue(data.UserID.ValueString() + "!" + data.TokenName.ValueString())
	data.Enable = accessBoolPointerValue(apiData.Enable)
	if apiData.Comment == "" {
		data.Comment = types.StringNull()
	} else {
		data.Comment = types.StringValue(apiData.Comment)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
