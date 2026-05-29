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

var _ datasource.DataSource = &DatastoreDataSource{}

func NewDatastoreDataSource() datasource.DataSource { return &DatastoreDataSource{} }

type DatastoreDataSource struct{ client *proxmoxBackupServerClient }

type DatastoreDataSourceModel struct {
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
}

func (d *DatastoreDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = typeNamePrefix + "_datastore"
}

func (d *DatastoreDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a Proxmox Backup Server datastore configuration via `/config/datastore/{name}`.",
		Attributes: map[string]schema.Attribute{
			"name":                    schema.StringAttribute{MarkdownDescription: "Datastore name.", Required: true},
			"path":                    schema.StringAttribute{MarkdownDescription: "Datastore path.", Computed: true},
			"backend":                 schema.StringAttribute{MarkdownDescription: "Datastore backend config.", Computed: true},
			"backing_device":          schema.StringAttribute{MarkdownDescription: "UUID of the filesystem partition for removable datastores.", Computed: true},
			"comment":                 schema.StringAttribute{MarkdownDescription: "Comment.", Computed: true},
			"gc_schedule":             schema.StringAttribute{MarkdownDescription: "Garbage collection calendar event schedule.", Computed: true},
			"gc_on_unmount":           schema.BoolAttribute{MarkdownDescription: "Whether garbage collection runs before unmounting a removable datastore.", Computed: true},
			"prune_schedule":          schema.StringAttribute{MarkdownDescription: "Prune calendar event schedule.", Computed: true},
			"keep_last":               schema.Int64Attribute{MarkdownDescription: "Number of backups to keep.", Computed: true},
			"keep_hourly":             schema.Int64Attribute{MarkdownDescription: "Number of hourly backups to keep.", Computed: true},
			"keep_daily":              schema.Int64Attribute{MarkdownDescription: "Number of daily backups to keep.", Computed: true},
			"keep_weekly":             schema.Int64Attribute{MarkdownDescription: "Number of weekly backups to keep.", Computed: true},
			"keep_monthly":            schema.Int64Attribute{MarkdownDescription: "Number of monthly backups to keep.", Computed: true},
			"keep_yearly":             schema.Int64Attribute{MarkdownDescription: "Number of yearly backups to keep.", Computed: true},
			"verify_new":              schema.BoolAttribute{MarkdownDescription: "Whether new backups are verified right after completion.", Computed: true},
			"notify_user":             schema.StringAttribute{MarkdownDescription: "User ID for legacy sendmail notifications.", Computed: true},
			"notify":                  schema.StringAttribute{MarkdownDescription: "Datastore notification settings in Proxmox Backup Server property-string format.", Computed: true},
			"notification_mode":       schema.StringAttribute{MarkdownDescription: "Notification mode.", Computed: true},
			"notification_thresholds": schema.StringAttribute{MarkdownDescription: "Notification threshold settings in Proxmox Backup Server property-string format.", Computed: true},
			"counter_reset_schedule":  schema.StringAttribute{MarkdownDescription: "Notification threshold counter reset calendar event schedule.", Computed: true},
			"tuning":                  schema.StringAttribute{MarkdownDescription: "Datastore tuning options in Proxmox Backup Server property-string format.", Computed: true},
			"maintenance_mode":        schema.StringAttribute{MarkdownDescription: "Maintenance mode in Proxmox Backup Server property-string format.", Computed: true},
		},
	}
}

func (d *DatastoreDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DatastoreDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DatastoreDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiData datastoreAPIModel
	if err := d.client.get(ctx, "/config/datastore/"+urlPathEscape(data.Name.ValueString()), &apiData); err != nil {
		resp.Diagnostics.AddError("Read Datastore Failed", err.Error())
		return
	}

	setDatastoreDataSourceData(&data, apiData)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func setDatastoreDataSourceData(data *DatastoreDataSourceModel, apiData datastoreAPIModel) {
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
}
