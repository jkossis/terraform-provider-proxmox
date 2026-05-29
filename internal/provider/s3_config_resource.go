// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &S3ConfigResource{}
var _ resource.ResourceWithImportState = &S3ConfigResource{}

func NewS3ConfigResource() resource.Resource {
	return &S3ConfigResource{}
}

type S3ConfigResource struct {
	client *proxmoxBackupServerClient
}

type S3ConfigResourceModel struct {
	ID             types.String `tfsdk:"id"`
	AccessKey      types.String `tfsdk:"access_key"`
	SecretKey      types.String `tfsdk:"secret_key"`
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

type s3ConfigAPIModel struct {
	ID             string   `json:"id"`
	AccessKey      string   `json:"access-key"`
	SecretKey      string   `json:"secret-key,omitempty"`
	Endpoint       string   `json:"endpoint"`
	Port           *int64   `json:"port,omitempty"`
	Region         *string  `json:"region,omitempty"`
	Fingerprint    *string  `json:"fingerprint,omitempty"`
	PathStyle      *bool    `json:"path-style,omitempty"`
	RateIn         *string  `json:"rate-in,omitempty"`
	BurstIn        *string  `json:"burst-in,omitempty"`
	RateOut        *string  `json:"rate-out,omitempty"`
	BurstOut       *string  `json:"burst-out,omitempty"`
	ProviderQuirks []string `json:"provider-quirks,omitempty"`
	PutRateLimit   *int64   `json:"put-rate-limit,omitempty"`
	Delete         []string `json:"delete,omitempty"`
}

func (r *S3ConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_s3_config"
}

func (r *S3ConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server S3 client configuration via `/config/s3`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "ID to uniquely identify the S3 client config.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access_key": schema.StringAttribute{
				MarkdownDescription: "Access key for the S3 object store.",
				Required:            true,
			},
			"secret_key": schema.StringAttribute{
				MarkdownDescription: "Secret key for the S3 object store. Proxmox Backup Server does not return this value from read APIs, so Terraform preserves the configured value in state.",
				Required:            true,
				Sensitive:           true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Host name or IP address of the S3 object store. Do not include a protocol scheme or port; use `port` for non-default ports.",
				Required:            true,
				Validators: []validator.String{
					s3EndpointValidator{},
				},
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "Port to access the S3 object store.",
				Optional:            true,
				Validators: []validator.Int64{
					int64RangeValidator{min: 1, max: 65535, description: "port must be between 1 and 65535"},
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region to access the S3 object store.",
				Optional:            true,
			},
			"fingerprint": schema.StringAttribute{
				MarkdownDescription: "X509 certificate fingerprint (SHA256).",
				Optional:            true,
			},
			"path_style": schema.BoolAttribute{
				MarkdownDescription: "Use path style bucket addressing over vhost style.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"rate_in": schema.StringAttribute{
				MarkdownDescription: "Incoming rate limit as a byte size with optional unit.",
				Optional:            true,
			},
			"burst_in": schema.StringAttribute{
				MarkdownDescription: "Incoming burst limit as a byte size with optional unit.",
				Optional:            true,
			},
			"rate_out": schema.StringAttribute{
				MarkdownDescription: "Outgoing rate limit as a byte size with optional unit.",
				Optional:            true,
			},
			"burst_out": schema.StringAttribute{
				MarkdownDescription: "Outgoing burst limit as a byte size with optional unit.",
				Optional:            true,
			},
			"provider_quirks": schema.ListAttribute{
				MarkdownDescription: "Provider-specific feature implementation quirks. Supported values include `skip-if-none-match-header` and `delete-objects-via-delete-object`.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					stringListAllowedValuesValidator{allowed: map[string]struct{}{
						"skip-if-none-match-header":        {},
						"delete-objects-via-delete-object": {},
					}},
				},
			},
			"put_rate_limit": schema.Int64Attribute{
				MarkdownDescription: "Rate limit for PUT requests, in requests per second. Proxmox Backup Server does not allow this value to be deleted after it is set, so Terraform preserves the prior state value when omitted from configuration.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Int64{
					int64RangeValidator{min: 1, max: 9223372036854775807, description: "put_rate_limit must be greater than 0"},
				},
			},
		},
	}
}

func (r *S3ConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*proxmoxBackupServerClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *proxmoxBackupServerClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *S3ConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data S3ConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, diags := s3ConfigPayload(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.post(ctx, "/config/s3", payload, nil); err != nil {
		resp.Diagnostics.AddError("Create S3 Config Failed", err.Error())
		return
	}

	if err := r.readS3Config(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read S3 Config Failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *S3ConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data S3ConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readS3Config(ctx, &data); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read S3 Config Failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *S3ConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan S3ConfigResourceModel
	var state S3ConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, diags := s3ConfigPayload(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload.Delete = s3ConfigDeletedFields(plan, state)

	if err := r.client.put(ctx, "/config/s3/"+urlPathEscape(plan.ID.ValueString()), payload); err != nil {
		resp.Diagnostics.AddError("Update S3 Config Failed", err.Error())
		return
	}

	if err := r.readS3Config(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read S3 Config Failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *S3ConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data S3ConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.delete(ctx, "/config/s3/"+urlPathEscape(data.ID.ValueString())); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			return
		}
		resp.Diagnostics.AddError("Delete S3 Config Failed", err.Error())
	}
}

func (r *S3ConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *S3ConfigResource) readS3Config(ctx context.Context, data *S3ConfigResourceModel) error {
	configuredProviderQuirks := data.ProviderQuirks
	configuredRateIn := data.RateIn
	configuredBurstIn := data.BurstIn
	configuredRateOut := data.RateOut
	configuredBurstOut := data.BurstOut

	var apiData s3ConfigAPIModel
	if err := r.client.get(ctx, "/config/s3/"+urlPathEscape(data.ID.ValueString()), &apiData); err != nil {
		return err
	}

	data.ID = types.StringValue(apiData.ID)
	data.AccessKey = types.StringValue(apiData.AccessKey)
	data.Endpoint = types.StringValue(apiData.Endpoint)
	data.Port = int64PointerValue(apiData.Port)
	data.Region = stringPointerValue(apiData.Region)
	data.Fingerprint = stringPointerValue(apiData.Fingerprint)
	data.PathStyle = boolPointerValue(apiData.PathStyle, false)
	data.RateIn = normalizedStringPointerValue(apiData.RateIn, configuredRateIn)
	data.BurstIn = normalizedStringPointerValue(apiData.BurstIn, configuredBurstIn)
	data.RateOut = normalizedStringPointerValue(apiData.RateOut, configuredRateOut)
	data.BurstOut = normalizedStringPointerValue(apiData.BurstOut, configuredBurstOut)
	if apiData.ProviderQuirks == nil && !configuredProviderQuirks.IsNull() && !configuredProviderQuirks.IsUnknown() {
		data.ProviderQuirks = configuredProviderQuirks
	} else {
		data.ProviderQuirks = stringListValue(apiData.ProviderQuirks)
	}
	data.PutRateLimit = int64PointerValue(apiData.PutRateLimit)

	return nil
}

func s3ConfigPayload(ctx context.Context, data S3ConfigResourceModel) (s3ConfigAPIModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	quirks, listDiags := stringListElements(ctx, data.ProviderQuirks)
	diags.Append(listDiags...)

	return s3ConfigAPIModel{
		ID:             data.ID.ValueString(),
		AccessKey:      data.AccessKey.ValueString(),
		SecretKey:      data.SecretKey.ValueString(),
		Endpoint:       data.Endpoint.ValueString(),
		Port:           int64Pointer(data.Port),
		Region:         stringPointer(data.Region),
		Fingerprint:    stringPointer(data.Fingerprint),
		PathStyle:      boolPointer(data.PathStyle),
		RateIn:         stringPointer(data.RateIn),
		BurstIn:        stringPointer(data.BurstIn),
		RateOut:        stringPointer(data.RateOut),
		BurstOut:       stringPointer(data.BurstOut),
		ProviderQuirks: quirks,
		PutRateLimit:   int64Pointer(data.PutRateLimit),
	}, diags
}

func s3ConfigDeletedFields(plan, state S3ConfigResourceModel) []string {
	var deleted []string
	if plan.Port.IsNull() && !state.Port.IsNull() {
		deleted = append(deleted, "port")
	}
	if plan.Region.IsNull() && !state.Region.IsNull() {
		deleted = append(deleted, "region")
	}
	if plan.Fingerprint.IsNull() && !state.Fingerprint.IsNull() {
		deleted = append(deleted, "fingerprint")
	}
	if plan.RateIn.IsNull() && !state.RateIn.IsNull() {
		deleted = append(deleted, "rate-in")
	}
	if plan.BurstIn.IsNull() && !state.BurstIn.IsNull() {
		deleted = append(deleted, "burst-in")
	}
	if plan.RateOut.IsNull() && !state.RateOut.IsNull() {
		deleted = append(deleted, "rate-out")
	}
	if plan.BurstOut.IsNull() && !state.BurstOut.IsNull() {
		deleted = append(deleted, "burst-out")
	}
	if providerQuirksShouldBeDeleted(plan.ProviderQuirks, state.ProviderQuirks) {
		deleted = append(deleted, "provider-quirks")
	}

	return deleted
}

func providerQuirksShouldBeDeleted(plan, state types.List) bool {
	if state.IsNull() || state.IsUnknown() {
		return false
	}
	if plan.IsNull() {
		return true
	}
	if plan.IsUnknown() {
		return false
	}

	return len(plan.Elements()) == 0 && len(state.Elements()) > 0
}

func stringPointer(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := value.ValueString()
	return &v
}

func int64Pointer(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := value.ValueInt64()
	return &v
}

func boolPointer(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := value.ValueBool()
	return &v
}

func stringPointerValue(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func normalizedStringPointerValue(value *string, configured types.String) types.String {
	if value == nil {
		return types.StringNull()
	}
	if !configured.IsNull() && !configured.IsUnknown() && strings.ReplaceAll(configured.ValueString(), " ", "") == strings.ReplaceAll(*value, " ", "") {
		return configured
	}
	return types.StringValue(*value)
}

func int64PointerValue(value *int64) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*value)
}

func boolPointerValue(value *bool, defaultValue bool) types.Bool {
	if value == nil {
		return types.BoolValue(defaultValue)
	}
	return types.BoolValue(*value)
}

func stringListValue(values []string) types.List {
	if values == nil {
		return types.ListNull(types.StringType)
	}
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	list, _ := types.ListValue(types.StringType, elements)
	return list
}

func stringListElements(ctx context.Context, value types.List) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}

	var values []string
	diags.Append(value.ElementsAs(ctx, &values, false)...)
	return values, diags
}

func urlPathEscape(value string) string {
	return url.PathEscape(value)
}
