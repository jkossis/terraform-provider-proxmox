// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerTypeName = "proxmox"
	typeNamePrefix   = "proxmox_backup_server"
)

// Ensure ProxmoxBackupServerProvider satisfies the Terraform provider interface.
var _ provider.Provider = &ProxmoxBackupServerProvider{}

// ProxmoxBackupServerProvider defines the provider implementation.
type ProxmoxBackupServerProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// ProxmoxBackupServerProviderModel describes the provider data model.
type ProxmoxBackupServerProviderModel struct {
	Endpoint    types.String `tfsdk:"endpoint"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	InsecureTLS types.Bool   `tfsdk:"insecure_tls"`
}

func (p *ProxmoxBackupServerProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = providerTypeName
	resp.Version = p.version
}

func (p *ProxmoxBackupServerProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for Proxmox Backup Server.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Proxmox Backup Server endpoint, for example `https://backup.example.com:8007`.",
				Required:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Proxmox Backup Server username, for example `root@pam`.",
				Required:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Proxmox Backup Server password used to request an authentication ticket.",
				Required:            true,
				Sensitive:           true,
			},
			"insecure_tls": schema.BoolAttribute{
				MarkdownDescription: "Skip TLS certificate verification. This should only be used for lab or self-signed Proxmox Backup Server installations.",
				Optional:            true,
			},
		},
	}
}

func (p *ProxmoxBackupServerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ProxmoxBackupServerProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	client, err := newProxmoxBackupServerClient(
		data.Endpoint.ValueString(),
		data.Username.ValueString(),
		data.Password.ValueString(),
		data.InsecureTLS.ValueBool(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Proxmox Backup Server Client Configuration", fmt.Sprintf("Unable to configure Proxmox Backup Server client: %s", err))
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *ProxmoxBackupServerProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewACLResource,
		NewDatastoreResource,
		NewS3ConfigResource,
		NewUserResource,
		NewUserTokenResource,
	}
}

func (p *ProxmoxBackupServerProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewACLDataSource,
		NewDatastoreDataSource,
		NewS3ConfigDataSource,
		NewUserDataSource,
		NewUserTokenDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ProxmoxBackupServerProvider{
			version: version,
		}
	}
}
