// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &VerifyJobResource{}
var _ resource.ResourceWithImportState = &VerifyJobResource{}
var _ resource.ResourceWithValidateConfig = &VerifyJobResource{}

func NewVerifyJobResource() resource.Resource { return &VerifyJobResource{} }

type VerifyJobResource struct{ client *proxmoxBackupServerClient }

type VerifyJobResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Store          types.String `tfsdk:"store"`
	Schedule       types.String `tfsdk:"schedule"`
	Comment        types.String `tfsdk:"comment"`
	Namespace      types.String `tfsdk:"ns"`
	MaxDepth       types.Int64  `tfsdk:"max_depth"`
	IgnoreVerified types.Bool   `tfsdk:"ignore_verified"`
	OutdatedAfter  types.Int64  `tfsdk:"outdated_after"`
	ReadThreads    types.Int64  `tfsdk:"read_threads"`
	VerifyThreads  types.Int64  `tfsdk:"verify_threads"`
}

type verifyJobAPIModel struct {
	ID             string  `json:"id"`
	Store          string  `json:"store"`
	Schedule       *string `json:"schedule,omitempty"`
	Comment        *string `json:"comment,omitempty"`
	Namespace      *string `json:"ns,omitempty"`
	MaxDepth       *int64  `json:"max-depth,omitempty"`
	IgnoreVerified *bool   `json:"ignore-verified,omitempty"`
	OutdatedAfter  *int64  `json:"outdated-after,omitempty"`
	ReadThreads    *int64  `json:"read-threads,omitempty"`
	VerifyThreads  *int64  `json:"verify-threads,omitempty"`
}

func (r *VerifyJobResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_verify_job"
}

func (r *VerifyJobResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server verification job via `/config/verify`. Removing this resource removes only the job configuration, not backups or the datastore. Schedules use the PBS server's time zone.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{MarkdownDescription: "Unique verification job ID. Import using this ID.", Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"store":           schema.StringAttribute{MarkdownDescription: "Datastore to verify.", Required: true},
			"schedule":        schema.StringAttribute{MarkdownDescription: "PBS calendar event schedule. Omit to disable automatic runs while keeping the job.", Optional: true},
			"comment":         schema.StringAttribute{MarkdownDescription: "Non-empty comment without leading or trailing whitespace.", Optional: true},
			"ns":              schema.StringAttribute{MarkdownDescription: "Backup namespace. Omit to start at the root namespace.", Optional: true},
			"max_depth":       schema.Int64Attribute{MarkdownDescription: "Namespace recursion depth; zero verifies only the selected namespace. PBS defaults to full recursion when omitted.", Optional: true, Validators: []validator.Int64{int64RangeValidator{min: 0, max: 7, description: "namespace depth must be between 0 and 7"}}},
			"ignore_verified": schema.BoolAttribute{MarkdownDescription: "Skip already verified snapshots unless their verification is outdated. PBS defaults to true when omitted.", Optional: true},
			"outdated_after":  schema.Int64Attribute{MarkdownDescription: "Days before a successful verification becomes outdated. Use a positive value to periodically recheck old backups.", Optional: true, Validators: []validator.Int64{int64RangeValidator{min: 1, max: math.MaxInt64, description: "verification age must be positive"}}},
			"read_threads":    schema.Int64Attribute{MarkdownDescription: "Chunk reader threads (1–32). PBS defaults to 1 when omitted. Requires PBS support for verification thread settings.", Optional: true, Validators: []validator.Int64{int64RangeValidator{min: 1, max: 32, description: "reader thread count must be between 1 and 32"}}},
			"verify_threads":  schema.Int64Attribute{MarkdownDescription: "Verification threads (1–32). PBS defaults to 4 when omitted. Requires PBS support for verification thread settings.", Optional: true, Validators: []validator.Int64{int64RangeValidator{min: 1, max: 32, description: "verification thread count must be between 1 and 32"}}},
		},
	}
}

func (r *VerifyJobResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data VerifyJobResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, value := range map[string]types.String{"comment": data.Comment, "ns": data.Namespace} {
		if !value.IsNull() && !value.IsUnknown() && (value.ValueString() == "" || strings.TrimSpace(value.ValueString()) != value.ValueString()) {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid verification job value", "Omit this attribute or provide a non-empty value without leading or trailing whitespace.")
		}
	}
}

func (r *VerifyJobResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*proxmoxBackupServerClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected resource configure type", fmt.Sprintf("Expected *proxmoxBackupServerClient, got: %T. Please report this issue to the provider developers.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *VerifyJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VerifyJobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.withConfigMutation(ctx, func(ctx context.Context) (*string, error) {
		var jobs []verifyJobAPIModel
		return r.client.getWithDigest(ctx, "/config/verify", &jobs)
	}, func(ctx context.Context, _ *string) error {
		// Collection POST does not accept a digest.
		return r.client.postForm(ctx, "/config/verify", verifyJobForm(data, true), nil)
	})
	if err != nil {
		resp.Diagnostics.AddError("Create verification job failed", err.Error())
		return
	}
	// Retain ownership if the subsequent read fails after successful creation.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readVerifyJob(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read verification job failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VerifyJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VerifyJobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readVerifyJob(ctx, &data); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read verification job failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VerifyJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state VerifyJobResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiPath := "/config/verify/" + urlPathEscape(plan.ID.ValueString())
	err := r.client.withConfigMutation(ctx, func(ctx context.Context) (*string, error) {
		var job verifyJobAPIModel
		return r.client.getWithDigest(ctx, apiPath, &job)
	}, func(ctx context.Context, digest *string) error {
		form, previous := verifyJobForm(plan, false), verifyJobForm(state, false)
		for _, key := range []string{"schedule", "comment", "ns", "max-depth", "ignore-verified", "outdated-after", "read-threads", "verify-threads"} {
			if previous.Has(key) && !form.Has(key) {
				form.Add("delete", key)
			}
		}
		setFreshDigest(form, digest)
		return r.client.putForm(ctx, apiPath, form)
	})
	if err != nil {
		resp.Diagnostics.AddError("Update verification job failed", err.Error())
		return
	}
	if err := r.readVerifyJob(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read verification job failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VerifyJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VerifyJobResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiPath := "/config/verify/" + urlPathEscape(data.ID.ValueString())
	err := r.client.withConfigMutation(ctx, func(ctx context.Context) (*string, error) {
		var job verifyJobAPIModel
		return r.client.getWithDigest(ctx, apiPath, &job)
	}, func(ctx context.Context, digest *string) error {
		form := url.Values{}
		setFreshDigest(form, digest)
		return r.client.deleteForm(ctx, apiPath, form)
	})
	if err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			return
		}
		resp.Diagnostics.AddError("Delete verification job failed", err.Error())
	}
}

func (r *VerifyJobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *VerifyJobResource) readVerifyJob(ctx context.Context, data *VerifyJobResourceModel) error {
	var job verifyJobAPIModel
	if err := r.client.get(ctx, "/config/verify/"+urlPathEscape(data.ID.ValueString()), &job); err != nil {
		return err
	}
	data.ID = types.StringValue(job.ID)
	data.Store = types.StringValue(job.Store)
	data.Schedule = stringPointerValue(job.Schedule)
	data.Comment = stringPointerValue(job.Comment)
	data.Namespace = stringPointerValue(job.Namespace)
	data.MaxDepth = int64PointerValue(job.MaxDepth)
	data.IgnoreVerified = boolPointerNullValue(job.IgnoreVerified)
	data.OutdatedAfter = int64PointerValue(job.OutdatedAfter)
	data.ReadThreads = int64PointerValue(job.ReadThreads)
	data.VerifyThreads = int64PointerValue(job.VerifyThreads)
	return nil
}

func verifyJobForm(data VerifyJobResourceModel, includeID bool) url.Values {
	form := url.Values{}
	if includeID {
		form.Set("id", data.ID.ValueString())
	}
	form.Set("store", data.Store.ValueString())
	setStringFormPointer(form, "schedule", stringPointer(data.Schedule))
	setStringFormPointer(form, "comment", stringPointer(data.Comment))
	setStringFormPointer(form, "ns", stringPointer(data.Namespace))
	setInt64FormPointer(form, "max-depth", int64Pointer(data.MaxDepth))
	setBoolFormPointer(form, "ignore-verified", boolPointer(data.IgnoreVerified))
	setInt64FormPointer(form, "outdated-after", int64Pointer(data.OutdatedAfter))
	setInt64FormPointer(form, "read-threads", int64Pointer(data.ReadThreads))
	setInt64FormPointer(form, "verify-threads", int64Pointer(data.VerifyThreads))
	return form
}
