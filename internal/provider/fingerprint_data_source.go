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

var _ datasource.DataSource = &FingerprintDataSource{}

func NewFingerprintDataSource() datasource.DataSource { return &FingerprintDataSource{} }

type FingerprintDataSource struct{ client *proxmoxBackupServerClient }

type FingerprintDataSourceModel struct {
	Fingerprint types.String `tfsdk:"fingerprint"`
}

func (d *FingerprintDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_fingerprint"
}

func (d *FingerprintDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Hashes the first (leaf) TLS certificate presented by the configured HTTPS endpoint using SHA-256. It bypasses certificate verification for trust-on-first-use (TOFU) discovery. The result may be a reverse-proxy certificate rather than a PBS node certificate.",
		Attributes: map[string]schema.Attribute{
			"fingerprint": schema.StringAttribute{
				MarkdownDescription: "SHA-256 hash of the first (leaf) TLS certificate presented by the configured HTTPS endpoint. It bypasses certificate verification for trust-on-first-use (TOFU) discovery, and the result may be a reverse-proxy certificate rather than a PBS node certificate. Formatted as uppercase colon-separated hex.",
				Computed:            true,
			},
		},
	}
}

func (d *FingerprintDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *FingerprintDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data FingerprintDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	fingerprint, err := d.client.certificateFingerprint(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Read Fingerprint Failed", err.Error())
		return
	}
	data.Fingerprint = types.StringValue(fingerprint)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
