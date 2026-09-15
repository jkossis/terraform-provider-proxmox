// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerTypeName = "proxmox"
	typeNamePrefix   = "proxmox_backup_server"

	proxmoxEndpointEnv    = "PROXMOX_ENDPOINT"
	proxmoxUsernameEnv    = "PROXMOX_USERNAME"
	proxmoxPasswordEnv    = "PROXMOX_PASSWORD"
	proxmoxInsecureTLSEnv = "PROXMOX_INSECURE_TLS"
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

type providerConfig struct {
	endpoint    string
	username    string
	password    string
	insecureTLS bool
}

type providerStringConfigSource struct {
	value       types.String
	attribute   string
	envVar      string
	displayName string
}

type providerBoolConfigSource struct {
	value       types.Bool
	attribute   string
	envVar      string
	displayName string
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
				MarkdownDescription: "Proxmox Backup Server endpoint, for example `https://backup.example.com:8007`. Can also be set with the " + proxmoxEndpointEnv + " environment variable.",
				Optional:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Proxmox Backup Server username, for example `root@pam`. Can also be set with the " + proxmoxUsernameEnv + " environment variable.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Proxmox Backup Server password used to request an authentication ticket. Can also be set with the " + proxmoxPasswordEnv + " environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"insecure_tls": schema.BoolAttribute{
				MarkdownDescription: "Skip TLS certificate verification. This should only be used for lab or self-signed Proxmox Backup Server installations. Can also be set with the " + proxmoxInsecureTLSEnv + " environment variable.",
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

	config, diags := providerConfigFrom(data, os.Getenv)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := newProxmoxBackupServerClient(
		config.endpoint,
		config.username,
		config.password,
		config.insecureTLS,
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Proxmox Backup Server Client Configuration", fmt.Sprintf("Unable to configure Proxmox Backup Server client: %s", err))
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func providerConfigFrom(data ProxmoxBackupServerProviderModel, lookupEnv func(string) string) (providerConfig, diag.Diagnostics) {
	var diags diag.Diagnostics

	config := providerConfig{
		endpoint: stringConfigValue(providerStringConfigSource{
			value:       data.Endpoint,
			attribute:   "endpoint",
			envVar:      proxmoxEndpointEnv,
			displayName: "Proxmox Backup Server Endpoint",
		}, lookupEnv, &diags),
		username: stringConfigValue(providerStringConfigSource{
			value:       data.Username,
			attribute:   "username",
			envVar:      proxmoxUsernameEnv,
			displayName: "Proxmox Backup Server Username",
		}, lookupEnv, &diags),
		password: stringConfigValue(providerStringConfigSource{
			value:       data.Password,
			attribute:   "password",
			envVar:      proxmoxPasswordEnv,
			displayName: "Proxmox Backup Server Password",
		}, lookupEnv, &diags),
	}
	config.insecureTLS = boolConfigValue(providerBoolConfigSource{
		value:       data.InsecureTLS,
		attribute:   "insecure_tls",
		envVar:      proxmoxInsecureTLSEnv,
		displayName: "Proxmox Backup Server Insecure TLS",
	}, lookupEnv, &diags)

	return config, diags
}

func stringConfigValue(source providerStringConfigSource, lookupEnv func(string) string, diags *diag.Diagnostics) string {
	if source.value.IsUnknown() {
		addUnknownProviderConfigDiagnostic(source.attribute, source.displayName, diags)
		return ""
	}

	if !source.value.IsNull() {
		value := source.value.ValueString()
		if value == "" {
			addMissingProviderConfigDiagnostic(source, diags)
		}
		return value
	}

	value := lookupEnv(source.envVar)
	if value == "" {
		addMissingProviderConfigDiagnostic(source, diags)
	}
	return value
}

func boolConfigValue(source providerBoolConfigSource, lookupEnv func(string) string, diags *diag.Diagnostics) bool {
	if source.value.IsUnknown() {
		addUnknownProviderConfigDiagnostic(source.attribute, source.displayName, diags)
		return false
	}

	if !source.value.IsNull() {
		return source.value.ValueBool()
	}

	value := lookupEnv(source.envVar)
	if value == "" {
		return false
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		diags.AddAttributeError(
			path.Root(source.attribute),
			"Invalid "+source.displayName,
			fmt.Sprintf("Set the %s provider attribute to a boolean value or the %s environment variable to a valid boolean string: %s.", source.attribute, source.envVar, err),
		)
		return false
	}

	return parsed
}

func addMissingProviderConfigDiagnostic(source providerStringConfigSource, diags *diag.Diagnostics) {
	diags.AddAttributeError(
		path.Root(source.attribute),
		"Missing "+source.displayName,
		fmt.Sprintf("Set the %s provider attribute or the %s environment variable.", source.attribute, source.envVar),
	)
}

func addUnknownProviderConfigDiagnostic(attribute, displayName string, diags *diag.Diagnostics) {
	diags.AddAttributeError(
		path.Root(attribute),
		"Unknown "+displayName,
		fmt.Sprintf("The %s provider attribute must be known during provider configuration.", attribute),
	)
}

func (p *ProxmoxBackupServerProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewACLResource,
		NewDatastoreResource,
		NewOpenIDRealmResource,
		NewS3ConfigResource,
		NewUserResource,
		NewUserTokenResource,
		NewVerifyJobResource,
	}
}

func (p *ProxmoxBackupServerProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewACLDataSource,
		NewDatastoreDataSource,
		NewFingerprintDataSource,
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
