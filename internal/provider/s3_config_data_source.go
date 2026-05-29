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

var _ datasource.DataSource = &S3ConfigDataSource{}

func NewS3ConfigDataSource() datasource.DataSource {
	return &S3ConfigDataSource{}
}

type S3ConfigDataSource struct {
	client *proxmoxBackupServerClient
}

type S3ConfigDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	AccessKey      types.String `tfsdk:"access_key"`
	Endpoint       types.String `tfsdk:"endpoint"`
	Port           types.Int64  `tfsdk:"port"`
	Region         types.String `tfsdk:"region"`
	Fingerprint    types.String `tfsdk:"fingerprint"`
	PathStyle      types.Bool   `tfsdk:"path_style"`
	RateIn         types.String `tfsdk:"rate_in"`
	BurstIn        types.String `tfsdk:"burst_in"`
	RateOut        types.String `tfsdk:"rate_out"`
	BurstOut       types.String `tfsdk:"burst_out"`
	ProviderQuirks types.List   `tfsdk:"provider_quirks"`
	PutRateLimit   types.Int64  `tfsdk:"put_rate_limit"`
}

func (d *S3ConfigDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_s3_config"
}

func (d *S3ConfigDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Proxmox Backup Server S3 client configuration via `/config/s3/{id}`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "ID of the S3 client config.",
				Required:            true,
			},
			"access_key": schema.StringAttribute{
				MarkdownDescription: "Access key for the S3 object store.",
				Computed:            true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Host name or IP address of the S3 object store, without a protocol scheme or port.",
				Computed:            true,
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "Port to access the S3 object store.",
				Computed:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region to access the S3 object store.",
				Computed:            true,
			},
			"fingerprint": schema.StringAttribute{
				MarkdownDescription: "X509 certificate fingerprint (SHA256).",
				Computed:            true,
			},
			"path_style": schema.BoolAttribute{
				MarkdownDescription: "Use path style bucket addressing over vhost style.",
				Computed:            true,
			},
			"rate_in": schema.StringAttribute{
				MarkdownDescription: "Incoming rate limit as a byte size with optional unit.",
				Computed:            true,
			},
			"burst_in": schema.StringAttribute{
				MarkdownDescription: "Incoming burst limit as a byte size with optional unit.",
				Computed:            true,
			},
			"rate_out": schema.StringAttribute{
				MarkdownDescription: "Outgoing rate limit as a byte size with optional unit.",
				Computed:            true,
			},
			"burst_out": schema.StringAttribute{
				MarkdownDescription: "Outgoing burst limit as a byte size with optional unit.",
				Computed:            true,
			},
			"provider_quirks": schema.ListAttribute{
				MarkdownDescription: "Provider-specific feature implementation quirks.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"put_rate_limit": schema.Int64Attribute{
				MarkdownDescription: "Rate limit for PUT requests, in requests per second.",
				Computed:            true,
			},
		},
	}
}

func (d *S3ConfigDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*proxmoxBackupServerClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *proxmoxBackupServerClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *S3ConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data S3ConfigDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiData s3ConfigAPIModel
	if err := d.client.get(ctx, "/config/s3/"+urlPathEscape(data.ID.ValueString()), &apiData); err != nil {
		resp.Diagnostics.AddError("Read S3 Config Failed", err.Error())
		return
	}

	data.ID = types.StringValue(apiData.ID)
	data.AccessKey = types.StringValue(apiData.AccessKey)
	data.Endpoint = types.StringValue(apiData.Endpoint)
	data.Port = int64PointerValue(apiData.Port)
	data.Region = stringPointerValue(apiData.Region)
	data.Fingerprint = stringPointerValue(apiData.Fingerprint)
	data.PathStyle = boolPointerValue(apiData.PathStyle, false)
	data.RateIn = stringPointerValue(apiData.RateIn)
	data.BurstIn = stringPointerValue(apiData.BurstIn)
	data.RateOut = stringPointerValue(apiData.RateOut)
	data.BurstOut = stringPointerValue(apiData.BurstOut)
	data.ProviderQuirks = stringListValue(apiData.ProviderQuirks)
	data.PutRateLimit = int64PointerValue(apiData.PutRateLimit)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
