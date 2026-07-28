// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
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

const defaultOpenIDRealmScopes = "email profile"

var _ resource.Resource = &OpenIDRealmResource{}
var _ resource.ResourceWithImportState = &OpenIDRealmResource{}

func NewOpenIDRealmResource() resource.Resource { return &OpenIDRealmResource{} }

type OpenIDRealmResource struct {
	client *proxmoxBackupServerClient
}

type OpenIDRealmResourceModel struct {
	Realm         types.String `tfsdk:"realm"`
	IssuerURL     types.String `tfsdk:"issuer_url"`
	ClientID      types.String `tfsdk:"client_id"`
	ClientKey     types.String `tfsdk:"client_key"`
	Scopes        types.String `tfsdk:"scopes"`
	ACRValues     types.String `tfsdk:"acr_values"`
	Prompt        types.String `tfsdk:"prompt"`
	Comment       types.String `tfsdk:"comment"`
	AutoCreate    types.Bool   `tfsdk:"autocreate"`
	UsernameClaim types.String `tfsdk:"username_claim"`
	ID            types.String `tfsdk:"id"`
	Digest        types.String `tfsdk:"digest"`
}

func (r *OpenIDRealmResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_realm_openid"
}

func (r *OpenIDRealmResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server OpenID realm via `/config/access/openid`.",
		Attributes: map[string]schema.Attribute{
			"realm": schema.StringAttribute{
				MarkdownDescription: "OpenID realm name.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"issuer_url": schema.StringAttribute{
				MarkdownDescription: "OpenID provider issuer URL.",
				Required:            true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "OpenID client ID.",
				Required:            true,
			},
			"client_key": schema.StringAttribute{
				MarkdownDescription: "OpenID client key. Proxmox Backup Server may not return this value from its read API, so Terraform preserves the configured value.",
				Optional:            true,
				Sensitive:           true,
			},
			"scopes": schema.StringAttribute{
				MarkdownDescription: "OpenID scopes. Proxmox Backup Server defaults this to `email profile`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(defaultOpenIDRealmScopes),
			},
			"acr_values": schema.StringAttribute{
				MarkdownDescription: "OpenID Authentication Context Class Reference values.",
				Optional:            true,
			},
			"prompt": schema.StringAttribute{
				MarkdownDescription: "OpenID prompt value.",
				Optional:            true,
			},
			"comment": schema.StringAttribute{
				MarkdownDescription: "OpenID realm comment.",
				Optional:            true,
				Validators:          []validator.String{openIDCommentValidator{}},
			},
			"autocreate": schema.BoolAttribute{
				MarkdownDescription: "Whether to automatically create users on first OpenID login. Proxmox Backup Server defaults this to false.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"username_claim": schema.StringAttribute{
				MarkdownDescription: "OpenID claim used as the Proxmox Backup Server username.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "OpenID realm name used as the resource ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"digest": schema.StringAttribute{
				MarkdownDescription: "Configuration digest used by Proxmox Backup Server for optimistic concurrency.",
				Computed:            true,
			},
		},
	}
}

func (r *OpenIDRealmResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OpenIDRealmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OpenIDRealmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(data.Realm.ValueString())
	if err := r.client.postForm(ctx, "/config/access/openid", openIDRealmForm(data, true), nil); err != nil {
		resp.Diagnostics.AddError("Create OpenID Realm Failed", openIDRealmError(err, data.ClientKey))
		return
	}
	recoveryState := openIDRealmRecoveryState(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &recoveryState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readOpenIDRealmWithDigest(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read OpenID Realm Failed", openIDRealmError(err, data.ClientKey))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OpenIDRealmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OpenIDRealmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readOpenIDRealmWithDigest(ctx, &data); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read OpenID Realm Failed", openIDRealmError(err, data.ClientKey))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OpenIDRealmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OpenIDRealmResourceModel
	var state OpenIDRealmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	form := openIDRealmForm(plan, false)
	for _, field := range openIDRealmDeletedFields(plan, state) {
		form.Add("delete", field)
	}
	setOpenIDDigest(form, state.Digest)
	if err := r.client.putForm(ctx, "/config/access/openid/"+urlPathEscape(plan.Realm.ValueString()), form); err != nil {
		resp.Diagnostics.AddError("Update OpenID Realm Failed", openIDRealmError(err, plan.ClientKey, state.ClientKey))
		return
	}
	if err := r.readOpenIDRealmWithDigest(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read OpenID Realm Failed", openIDRealmError(err, plan.ClientKey, state.ClientKey))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *OpenIDRealmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OpenIDRealmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	form := url.Values{}
	setOpenIDDigest(form, data.Digest)
	if err := r.client.deleteForm(ctx, "/config/access/openid/"+urlPathEscape(data.Realm.ValueString()), form); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			return
		}
		resp.Diagnostics.AddError("Delete OpenID Realm Failed", openIDRealmError(err, data.ClientKey))
	}
}

func (r *OpenIDRealmResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *OpenIDRealmResource) readOpenIDRealm(ctx context.Context, data *OpenIDRealmResourceModel) error {
	return r.readOpenIDRealmWithDigest(ctx, data)
}

func (r *OpenIDRealmResource) readOpenIDRealmWithDigest(ctx context.Context, data *OpenIDRealmResourceModel) error {
	configuredClientKey := data.ClientKey
	realm := data.Realm.ValueString()
	if realm == "" {
		realm = data.ID.ValueString()
	}

	var apiData openIDRealmAPIModel
	digest, err := r.client.getWithDigest(ctx, "/config/access/openid/"+urlPathEscape(realm), &apiData)
	if err != nil {
		return err
	}
	if apiData.Realm != "" {
		realm = apiData.Realm
	}

	data.Realm = types.StringValue(realm)
	data.IssuerURL = types.StringValue(apiData.IssuerURL)
	data.ClientID = types.StringValue(apiData.ClientID)
	// PBS can omit or redact client-key on read. Never copy an API value into state.
	data.ClientKey = configuredClientKey
	if apiData.Scopes == nil {
		data.Scopes = types.StringValue(defaultOpenIDRealmScopes)
	} else {
		data.Scopes = types.StringValue(*apiData.Scopes)
	}
	data.ACRValues = openIDStringValue(apiData.ACRValues)
	data.Prompt = openIDStringValue(apiData.Prompt)
	data.Comment = openIDStringValue(apiData.Comment)
	if apiData.AutoCreate == nil {
		data.AutoCreate = types.BoolValue(false)
	} else {
		data.AutoCreate = types.BoolValue(bool(*apiData.AutoCreate))
	}
	data.UsernameClaim = openIDStringValue(apiData.UsernameClaim)
	data.ID = types.StringValue(realm)
	data.Digest = openIDDigestValue(digest)

	return nil
}

func openIDRealmForm(data OpenIDRealmResourceModel, includeRealm bool) url.Values {
	form := url.Values{}
	if includeRealm {
		setOpenIDRealmString(form, "realm", data.Realm)
	}
	setOpenIDRealmString(form, "issuer-url", data.IssuerURL)
	setOpenIDRealmString(form, "client-id", data.ClientID)
	setOpenIDRealmString(form, "client-key", data.ClientKey)
	setOpenIDRealmString(form, "scopes", data.Scopes)
	setOpenIDRealmString(form, "acr-values", data.ACRValues)
	setOpenIDRealmString(form, "prompt", data.Prompt)
	setOpenIDRealmString(form, "comment", data.Comment)
	if !data.AutoCreate.IsNull() && !data.AutoCreate.IsUnknown() {
		form.Set("autocreate", strconv.FormatBool(data.AutoCreate.ValueBool()))
	}
	if includeRealm {
		setOpenIDRealmString(form, "username-claim", data.UsernameClaim)
	}

	return form
}

func setOpenIDRealmString(form url.Values, name string, value types.String) {
	if !value.IsNull() && !value.IsUnknown() {
		form.Set(name, value.ValueString())
	}
}

func setOpenIDDigest(form url.Values, digest types.String) {
	if !digest.IsNull() && !digest.IsUnknown() && digest.ValueString() != "" {
		form.Set("digest", digest.ValueString())
	}
}

func openIDRealmRecoveryState(data OpenIDRealmResourceModel) OpenIDRealmResourceModel {
	recovery := data
	recovery.ID = types.StringValue(data.Realm.ValueString())
	recovery.Digest = types.StringNull()
	if recovery.Scopes.IsUnknown() {
		recovery.Scopes = types.StringNull()
	}
	if recovery.AutoCreate.IsUnknown() {
		recovery.AutoCreate = types.BoolNull()
	}
	if recovery.UsernameClaim.IsUnknown() {
		recovery.UsernameClaim = types.StringNull()
	}

	return recovery
}

func openIDRealmDeletedFields(plan, state OpenIDRealmResourceModel) []string {
	var deleted []string
	for _, field := range []struct {
		name  string
		plan  types.String
		state types.String
	}{
		{name: "client-key", plan: plan.ClientKey, state: state.ClientKey},
		{name: "scopes", plan: plan.Scopes, state: state.Scopes},
		{name: "acr-values", plan: plan.ACRValues, state: state.ACRValues},
		{name: "prompt", plan: plan.Prompt, state: state.Prompt},
		{name: "comment", plan: plan.Comment, state: state.Comment},
	} {
		if field.plan.IsNull() && !field.state.IsNull() && !field.state.IsUnknown() {
			deleted = append(deleted, field.name)
		}
	}
	if plan.AutoCreate.IsNull() && !state.AutoCreate.IsNull() && !state.AutoCreate.IsUnknown() {
		deleted = append(deleted, "autocreate")
	}

	return deleted
}

func openIDStringValue(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func openIDDigestValue(value *string) types.String {
	if value == nil || *value == "" {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func openIDRealmError(err error, clientKeys ...types.String) string {
	message := err.Error()
	for _, clientKey := range clientKeys {
		if !clientKey.IsNull() && !clientKey.IsUnknown() && clientKey.ValueString() != "" {
			for _, secret := range openIDRealmSecretVariants(clientKey.ValueString()) {
				message = strings.ReplaceAll(message, secret, "[REDACTED]")
			}
		}
	}
	return message
}

func openIDRealmSecretVariants(secret string) []string {
	variants := []string{secret, url.QueryEscape(secret), url.PathEscape(secret)}
	for _, value := range append([]string(nil), variants...) {
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		quoted := string(encoded)
		variants = append(variants, quoted, strings.Trim(quoted, `"`))
	}

	return variants
}
