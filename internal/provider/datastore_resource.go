// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &DatastoreResource{}
var _ resource.ResourceWithImportState = &DatastoreResource{}

func NewDatastoreResource() resource.Resource { return &DatastoreResource{} }

type DatastoreResource struct{ client *proxmoxBackupServerClient }

type DatastoreResourceModel struct {
	Name                   types.String `tfsdk:"name"`
	Path                   types.String `tfsdk:"path"`
	Backend                types.String `tfsdk:"backend"`
	BackingDevice          types.String `tfsdk:"backing_device"`
	Comment                types.String `tfsdk:"comment"`
	GCSchedule             types.String `tfsdk:"gc_schedule"`
	GCOnUnmount            types.Bool   `tfsdk:"gc_on_unmount"`
	PruneSchedule          types.String `tfsdk:"prune_schedule"`
	KeepLast               types.Int64  `tfsdk:"keep_last"`
	KeepHourly             types.Int64  `tfsdk:"keep_hourly"`
	KeepDaily              types.Int64  `tfsdk:"keep_daily"`
	KeepWeekly             types.Int64  `tfsdk:"keep_weekly"`
	KeepMonthly            types.Int64  `tfsdk:"keep_monthly"`
	KeepYearly             types.Int64  `tfsdk:"keep_yearly"`
	VerifyNew              types.Bool   `tfsdk:"verify_new"`
	NotifyUser             types.String `tfsdk:"notify_user"`
	Notify                 types.String `tfsdk:"notify"`
	NotificationMode       types.String `tfsdk:"notification_mode"`
	NotificationThresholds types.String `tfsdk:"notification_thresholds"`
	CounterResetSchedule   types.String `tfsdk:"counter_reset_schedule"`
	Tuning                 types.String `tfsdk:"tuning"`
	MaintenanceMode        types.String `tfsdk:"maintenance_mode"`
	ReuseDatastore         types.Bool   `tfsdk:"reuse_datastore"`
	OverwriteInUse         types.Bool   `tfsdk:"overwrite_in_use"`
}

type datastoreAPIModel struct {
	Name                   string   `json:"name"`
	Path                   string   `json:"path,omitempty"`
	Backend                *string  `json:"backend,omitempty"`
	BackingDevice          *string  `json:"backing-device,omitempty"`
	Comment                *string  `json:"comment,omitempty"`
	GCSchedule             *string  `json:"gc-schedule,omitempty"`
	GCOnUnmount            *bool    `json:"gc-on-unmount,omitempty"`
	PruneSchedule          *string  `json:"prune-schedule,omitempty"`
	KeepLast               *int64   `json:"keep-last,omitempty"`
	KeepHourly             *int64   `json:"keep-hourly,omitempty"`
	KeepDaily              *int64   `json:"keep-daily,omitempty"`
	KeepWeekly             *int64   `json:"keep-weekly,omitempty"`
	KeepMonthly            *int64   `json:"keep-monthly,omitempty"`
	KeepYearly             *int64   `json:"keep-yearly,omitempty"`
	VerifyNew              *bool    `json:"verify-new,omitempty"`
	NotifyUser             *string  `json:"notify-user,omitempty"`
	Notify                 *string  `json:"notify,omitempty"`
	NotificationMode       *string  `json:"notification-mode,omitempty"`
	NotificationThresholds *string  `json:"notification-thresholds,omitempty"`
	CounterResetSchedule   *string  `json:"counter-reset-schedule,omitempty"`
	Tuning                 *string  `json:"tuning,omitempty"`
	MaintenanceMode        *string  `json:"maintenance-mode,omitempty"`
	ReuseDatastore         *bool    `json:"reuse-datastore,omitempty"`
	OverwriteInUse         *bool    `json:"overwrite-in-use,omitempty"`
	Delete                 []string `json:"delete,omitempty"`
}

func (r *DatastoreResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_datastore"
}

func (r *DatastoreResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Proxmox Backup Server datastore configuration via `/config/datastore`.",
		Attributes: map[string]schema.Attribute{
			"name":                    schema.StringAttribute{MarkdownDescription: "Datastore name.", Required: true, PlanModifiers: replace},
			"path":                    schema.StringAttribute{MarkdownDescription: "Absolute path to the datastore directory, or relative on-device path for removable datastores.", Required: true, PlanModifiers: replace},
			"backend":                 schema.StringAttribute{MarkdownDescription: "Datastore backend config.", Optional: true, Computed: true, PlanModifiers: replace},
			"backing_device":          schema.StringAttribute{MarkdownDescription: "UUID of the filesystem partition for removable datastores.", Optional: true, Computed: true, PlanModifiers: replace},
			"comment":                 schema.StringAttribute{MarkdownDescription: "Comment.", Optional: true},
			"gc_schedule":             schema.StringAttribute{MarkdownDescription: "Run garbage collection job at the specified calendar event schedule.", Optional: true},
			"gc_on_unmount":           schema.BoolAttribute{MarkdownDescription: "Run garbage collection before unmounting a removable datastore.", Optional: true},
			"prune_schedule":          schema.StringAttribute{MarkdownDescription: "Run prune job at the specified calendar event schedule.", Optional: true},
			"keep_last":               schema.Int64Attribute{MarkdownDescription: "Number of backups to keep.", Optional: true},
			"keep_hourly":             schema.Int64Attribute{MarkdownDescription: "Number of hourly backups to keep.", Optional: true},
			"keep_daily":              schema.Int64Attribute{MarkdownDescription: "Number of daily backups to keep.", Optional: true},
			"keep_weekly":             schema.Int64Attribute{MarkdownDescription: "Number of weekly backups to keep.", Optional: true},
			"keep_monthly":            schema.Int64Attribute{MarkdownDescription: "Number of monthly backups to keep.", Optional: true},
			"keep_yearly":             schema.Int64Attribute{MarkdownDescription: "Number of yearly backups to keep.", Optional: true},
			"verify_new":              schema.BoolAttribute{MarkdownDescription: "Verify new backups right after completion.", Optional: true},
			"notify_user":             schema.StringAttribute{MarkdownDescription: "User ID for legacy sendmail notifications.", Optional: true},
			"notify":                  schema.StringAttribute{MarkdownDescription: "Datastore notification settings in Proxmox Backup Server property-string format, for example `gc=error,prune=always`.", Optional: true},
			"notification_mode":       schema.StringAttribute{MarkdownDescription: "Notification mode, either `legacy-sendmail` or `notification-system`.", Optional: true},
			"notification_thresholds": schema.StringAttribute{MarkdownDescription: "Notification threshold settings in Proxmox Backup Server property-string format.", Optional: true},
			"counter_reset_schedule":  schema.StringAttribute{MarkdownDescription: "Reset notification threshold counters at the specified calendar event schedule.", Optional: true},
			"tuning":                  schema.StringAttribute{MarkdownDescription: "Datastore tuning options in Proxmox Backup Server property-string format.", Optional: true},
			"maintenance_mode":        schema.StringAttribute{MarkdownDescription: "Maintenance mode in Proxmox Backup Server property-string format, for example `type=read-only,message=maintenance`.", Optional: true},
			"reuse_datastore":         schema.BoolAttribute{MarkdownDescription: "Re-use an existing datastore directory during creation.", Optional: true},
			"overwrite_in_use":        schema.BoolAttribute{MarkdownDescription: "Overwrite in-use marker during creation for S3-backed datastores.", Optional: true},
		},
	}
}

func (r *DatastoreResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*proxmoxBackupServerClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *proxmoxBackupServerClient, got: %T. Please report this issue to the provider developers.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *DatastoreResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DatastoreResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.post(ctx, "/config/datastore", datastorePayload(data), nil); err != nil {
		resp.Diagnostics.AddError("Create Datastore Failed", err.Error())
		return
	}
	if err := r.readDatastore(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Read Datastore Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatastoreResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DatastoreResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readDatastore(ctx, &data); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Datastore Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DatastoreResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DatastoreResourceModel
	var state DatastoreResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload := datastorePayload(plan)
	payload.Path = ""
	payload.Backend = nil
	payload.BackingDevice = nil
	payload.ReuseDatastore = nil
	payload.OverwriteInUse = nil
	payload.Delete = datastoreDeletedFields(plan, state)
	if err := r.client.put(ctx, "/config/datastore/"+urlPathEscape(plan.Name.ValueString()), payload); err != nil {
		resp.Diagnostics.AddError("Update Datastore Failed", err.Error())
		return
	}
	if err := r.readDatastore(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Read Datastore Failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DatastoreResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DatastoreResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.delete(ctx, "/config/datastore/"+urlPathEscape(data.Name.ValueString())); err != nil {
		var apiErr *proxmoxBackupServerAPIError
		if errors.As(err, &apiErr) && apiErr.notFound() {
			return
		}
		resp.Diagnostics.AddError("Delete Datastore Failed", err.Error())
	}
}

func (r *DatastoreResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *DatastoreResource) readDatastore(ctx context.Context, data *DatastoreResourceModel) error {
	var apiData datastoreAPIModel
	if err := r.client.get(ctx, "/config/datastore/"+urlPathEscape(data.Name.ValueString()), &apiData); err != nil {
		return err
	}
	setDatastoreData(data, apiData)
	return nil
}

func datastorePayload(data DatastoreResourceModel) datastoreAPIModel {
	return datastoreAPIModel{
		Name:                   data.Name.ValueString(),
		Path:                   data.Path.ValueString(),
		Backend:                stringPointer(data.Backend),
		BackingDevice:          stringPointer(data.BackingDevice),
		Comment:                stringPointer(data.Comment),
		GCSchedule:             stringPointer(data.GCSchedule),
		GCOnUnmount:            boolPointer(data.GCOnUnmount),
		PruneSchedule:          stringPointer(data.PruneSchedule),
		KeepLast:               int64Pointer(data.KeepLast),
		KeepHourly:             int64Pointer(data.KeepHourly),
		KeepDaily:              int64Pointer(data.KeepDaily),
		KeepWeekly:             int64Pointer(data.KeepWeekly),
		KeepMonthly:            int64Pointer(data.KeepMonthly),
		KeepYearly:             int64Pointer(data.KeepYearly),
		VerifyNew:              boolPointer(data.VerifyNew),
		NotifyUser:             stringPointer(data.NotifyUser),
		Notify:                 stringPointer(data.Notify),
		NotificationMode:       stringPointer(data.NotificationMode),
		NotificationThresholds: stringPointer(data.NotificationThresholds),
		CounterResetSchedule:   stringPointer(data.CounterResetSchedule),
		Tuning:                 stringPointer(data.Tuning),
		MaintenanceMode:        stringPointer(data.MaintenanceMode),
		ReuseDatastore:         boolPointer(data.ReuseDatastore),
		OverwriteInUse:         boolPointer(data.OverwriteInUse),
	}
}

func setDatastoreData(data *DatastoreResourceModel, apiData datastoreAPIModel) {
	configuredReuseDatastore := data.ReuseDatastore
	configuredOverwriteInUse := data.OverwriteInUse

	data.Name = types.StringValue(apiData.Name)
	data.Path = types.StringValue(apiData.Path)
	data.Backend = stringPointerValue(apiData.Backend)
	data.BackingDevice = stringPointerValue(apiData.BackingDevice)
	data.Comment = stringPointerValue(apiData.Comment)
	data.GCSchedule = stringPointerValue(apiData.GCSchedule)
	data.GCOnUnmount = boolPointerNullValue(apiData.GCOnUnmount)
	data.PruneSchedule = stringPointerValue(apiData.PruneSchedule)
	data.KeepLast = int64PointerValue(apiData.KeepLast)
	data.KeepHourly = int64PointerValue(apiData.KeepHourly)
	data.KeepDaily = int64PointerValue(apiData.KeepDaily)
	data.KeepWeekly = int64PointerValue(apiData.KeepWeekly)
	data.KeepMonthly = int64PointerValue(apiData.KeepMonthly)
	data.KeepYearly = int64PointerValue(apiData.KeepYearly)
	data.VerifyNew = boolPointerNullValue(apiData.VerifyNew)
	data.NotifyUser = stringPointerValue(apiData.NotifyUser)
	data.Notify = stringPointerValue(apiData.Notify)
	data.NotificationMode = stringPointerValue(apiData.NotificationMode)
	data.NotificationThresholds = stringPointerValue(apiData.NotificationThresholds)
	data.CounterResetSchedule = stringPointerValue(apiData.CounterResetSchedule)
	data.Tuning = stringPointerValue(apiData.Tuning)
	data.MaintenanceMode = stringPointerValue(apiData.MaintenanceMode)
	data.ReuseDatastore = configuredReuseDatastore
	data.OverwriteInUse = configuredOverwriteInUse
}

func datastoreDeletedFields(plan, state DatastoreResourceModel) []string {
	var deleted []string
	maybeDeleteString := func(name string, planValue, stateValue types.String) {
		if planValue.IsNull() && !stateValue.IsNull() {
			deleted = append(deleted, name)
		}
	}
	maybeDeleteInt64 := func(name string, planValue, stateValue types.Int64) {
		if planValue.IsNull() && !stateValue.IsNull() {
			deleted = append(deleted, name)
		}
	}
	maybeDeleteBool := func(name string, planValue, stateValue types.Bool) {
		if planValue.IsNull() && !stateValue.IsNull() {
			deleted = append(deleted, name)
		}
	}
	maybeDeleteString("comment", plan.Comment, state.Comment)
	maybeDeleteString("gc-schedule", plan.GCSchedule, state.GCSchedule)
	maybeDeleteBool("gc-on-unmount", plan.GCOnUnmount, state.GCOnUnmount)
	maybeDeleteString("prune-schedule", plan.PruneSchedule, state.PruneSchedule)
	maybeDeleteInt64("keep-last", plan.KeepLast, state.KeepLast)
	maybeDeleteInt64("keep-hourly", plan.KeepHourly, state.KeepHourly)
	maybeDeleteInt64("keep-daily", plan.KeepDaily, state.KeepDaily)
	maybeDeleteInt64("keep-weekly", plan.KeepWeekly, state.KeepWeekly)
	maybeDeleteInt64("keep-monthly", plan.KeepMonthly, state.KeepMonthly)
	maybeDeleteInt64("keep-yearly", plan.KeepYearly, state.KeepYearly)
	maybeDeleteBool("verify-new", plan.VerifyNew, state.VerifyNew)
	maybeDeleteString("notify-user", plan.NotifyUser, state.NotifyUser)
	maybeDeleteString("notify", plan.Notify, state.Notify)
	maybeDeleteString("notification-mode", plan.NotificationMode, state.NotificationMode)
	maybeDeleteString("tuning", plan.Tuning, state.Tuning)
	maybeDeleteString("maintenance-mode", plan.MaintenanceMode, state.MaintenanceMode)
	maybeDeleteString("notification-thresholds", plan.NotificationThresholds, state.NotificationThresholds)
	maybeDeleteString("counter-reset-schedule", plan.CounterResetSchedule, state.CounterResetSchedule)
	return deleted
}

func boolPointerNullValue(value *bool) types.Bool {
	if value == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*value)
}
