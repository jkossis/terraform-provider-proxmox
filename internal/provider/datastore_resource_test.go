// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDatastorePayloadIncludesOptionalFields(t *testing.T) {
	data := DatastoreResourceModel{
		Name:                   types.StringValue("backup"),
		Path:                   types.StringValue("/mnt/datastore/backup"),
		Backend:                types.StringValue("s3"),
		BackingDevice:          types.StringValue("12345678-1234-1234-1234-123456789abc"),
		Comment:                types.StringValue("Terraform managed"),
		GCSchedule:             types.StringValue("daily"),
		GCOnUnmount:            types.BoolValue(true),
		PruneSchedule:          types.StringValue("hourly"),
		KeepLast:               types.Int64Value(1),
		KeepHourly:             types.Int64Value(2),
		KeepDaily:              types.Int64Value(3),
		KeepWeekly:             types.Int64Value(4),
		KeepMonthly:            types.Int64Value(5),
		KeepYearly:             types.Int64Value(6),
		VerifyNew:              types.BoolValue(true),
		NotifyUser:             types.StringValue("root@pam"),
		Notify:                 types.StringValue("gc=error,prune=always"),
		NotificationMode:       types.StringValue("notification-system"),
		NotificationThresholds: types.StringValue("s3-get=100"),
		CounterResetSchedule:   types.StringValue("weekly"),
		Tuning:                 types.StringValue("sync-level=filesystem"),
		MaintenanceMode:        types.StringValue("type=read-only,message=maintenance"),
		ReuseDatastore:         types.BoolValue(true),
		OverwriteInUse:         types.BoolValue(true),
	}

	got := datastorePayload(data)
	if got.Name != "backup" {
		t.Fatalf("unexpected name: got %q", got.Name)
	}
	if got.Path != "/mnt/datastore/backup" {
		t.Fatalf("unexpected path: got %q", got.Path)
	}
	assertStringPointer(t, "backend", got.Backend, "s3")
	assertStringPointer(t, "backing-device", got.BackingDevice, "12345678-1234-1234-1234-123456789abc")
	assertStringPointer(t, "comment", got.Comment, "Terraform managed")
	assertStringPointer(t, "gc-schedule", got.GCSchedule, "daily")
	assertTruePointer(t, "gc-on-unmount", got.GCOnUnmount)
	assertStringPointer(t, "prune-schedule", got.PruneSchedule, "hourly")
	assertInt64Pointer(t, "keep-last", got.KeepLast, 1)
	assertInt64Pointer(t, "keep-hourly", got.KeepHourly, 2)
	assertInt64Pointer(t, "keep-daily", got.KeepDaily, 3)
	assertInt64Pointer(t, "keep-weekly", got.KeepWeekly, 4)
	assertInt64Pointer(t, "keep-monthly", got.KeepMonthly, 5)
	assertInt64Pointer(t, "keep-yearly", got.KeepYearly, 6)
	assertTruePointer(t, "verify-new", got.VerifyNew)
	assertStringPointer(t, "notify-user", got.NotifyUser, "root@pam")
	assertStringPointer(t, "notify", got.Notify, "gc=error,prune=always")
	assertStringPointer(t, "notification-mode", got.NotificationMode, "notification-system")
	assertStringPointer(t, "notification-thresholds", got.NotificationThresholds, "s3-get=100")
	assertStringPointer(t, "counter-reset-schedule", got.CounterResetSchedule, "weekly")
	assertStringPointer(t, "tuning", got.Tuning, "sync-level=filesystem")
	assertStringPointer(t, "maintenance-mode", got.MaintenanceMode, "type=read-only,message=maintenance")
	assertTruePointer(t, "reuse-datastore", got.ReuseDatastore)
	assertTruePointer(t, "overwrite-in-use", got.OverwriteInUse)
}

func TestDatastoreDeletedFields(t *testing.T) {
	plan := DatastoreResourceModel{
		Comment:                types.StringNull(),
		GCSchedule:             types.StringNull(),
		GCOnUnmount:            types.BoolNull(),
		PruneSchedule:          types.StringNull(),
		KeepLast:               types.Int64Null(),
		KeepHourly:             types.Int64Null(),
		KeepDaily:              types.Int64Null(),
		KeepWeekly:             types.Int64Null(),
		KeepMonthly:            types.Int64Null(),
		KeepYearly:             types.Int64Null(),
		VerifyNew:              types.BoolNull(),
		NotifyUser:             types.StringNull(),
		Notify:                 types.StringNull(),
		NotificationMode:       types.StringNull(),
		Tuning:                 types.StringNull(),
		MaintenanceMode:        types.StringNull(),
		NotificationThresholds: types.StringNull(),
		CounterResetSchedule:   types.StringNull(),
	}
	state := DatastoreResourceModel{
		Comment:                types.StringValue("comment"),
		GCSchedule:             types.StringValue("daily"),
		GCOnUnmount:            types.BoolValue(true),
		PruneSchedule:          types.StringValue("hourly"),
		KeepLast:               types.Int64Value(1),
		KeepHourly:             types.Int64Value(2),
		KeepDaily:              types.Int64Value(3),
		KeepWeekly:             types.Int64Value(4),
		KeepMonthly:            types.Int64Value(5),
		KeepYearly:             types.Int64Value(6),
		VerifyNew:              types.BoolValue(true),
		NotifyUser:             types.StringValue("root@pam"),
		Notify:                 types.StringValue("gc=error"),
		NotificationMode:       types.StringValue("notification-system"),
		Tuning:                 types.StringValue("sync-level=file"),
		MaintenanceMode:        types.StringValue("type=offline"),
		NotificationThresholds: types.StringValue("s3-get=100"),
		CounterResetSchedule:   types.StringValue("weekly"),
	}

	got := datastoreDeletedFields(plan, state)
	want := []string{
		"comment",
		"gc-schedule",
		"gc-on-unmount",
		"prune-schedule",
		"keep-last",
		"keep-hourly",
		"keep-daily",
		"keep-weekly",
		"keep-monthly",
		"keep-yearly",
		"verify-new",
		"notify-user",
		"notify",
		"notification-mode",
		"tuning",
		"maintenance-mode",
		"notification-thresholds",
		"counter-reset-schedule",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deleted fields: got %#v, want %#v", got, want)
	}
}

func TestSetDatastoreDataMapsAPIFieldsAndPreservesCreateOnlyFields(t *testing.T) {
	backend := "s3"
	comment := "Terraform managed"
	gcOnUnmount := true
	keepDaily := int64(7)
	apiData := datastoreAPIModel{
		Name:        "backup",
		Path:        "/mnt/datastore/backup",
		Backend:     &backend,
		Comment:     &comment,
		GCOnUnmount: &gcOnUnmount,
		KeepDaily:   &keepDaily,
	}
	data := DatastoreResourceModel{
		ReuseDatastore: types.BoolValue(true),
		OverwriteInUse: types.BoolValue(false),
	}

	setDatastoreData(&data, apiData)
	if got, want := data.Name.ValueString(), "backup"; got != want {
		t.Fatalf("unexpected name: got %q, want %q", got, want)
	}
	if got, want := data.Path.ValueString(), "/mnt/datastore/backup"; got != want {
		t.Fatalf("unexpected path: got %q, want %q", got, want)
	}
	if got, want := data.Backend.ValueString(), "s3"; got != want {
		t.Fatalf("unexpected backend: got %q, want %q", got, want)
	}
	if got, want := data.Comment.ValueString(), "Terraform managed"; got != want {
		t.Fatalf("unexpected comment: got %q, want %q", got, want)
	}
	if !data.GCOnUnmount.ValueBool() {
		t.Fatal("expected gc_on_unmount to be true")
	}
	if got, want := data.KeepDaily.ValueInt64(), int64(7); got != want {
		t.Fatalf("unexpected keep_daily: got %d, want %d", got, want)
	}
	if !data.ReuseDatastore.ValueBool() {
		t.Fatal("reuse_datastore was not preserved")
	}
	if data.OverwriteInUse.ValueBool() {
		t.Fatal("overwrite_in_use was not preserved")
	}
}

func TestReadDatastoreUsesConfigDatastoreEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"ticket":"ticket-value","CSRFPreventionToken":"csrf-value"}}`))
		case "/api2/json/config/datastore/backup":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"name":"backup","path":"/mnt/datastore/backup","comment":"Terraform managed","keep-daily":7,"verify-new":true}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	data := DatastoreResourceModel{Name: types.StringValue("backup")}
	resource := DatastoreResource{client: client}
	if err := resource.readDatastore(context.Background(), &data); err != nil {
		t.Fatalf("readDatastore returned error: %s", err)
	}
	if got, want := data.Path.ValueString(), "/mnt/datastore/backup"; got != want {
		t.Fatalf("unexpected path: got %q, want %q", got, want)
	}
	if got, want := data.Comment.ValueString(), "Terraform managed"; got != want {
		t.Fatalf("unexpected comment: got %q, want %q", got, want)
	}
	if got, want := data.KeepDaily.ValueInt64(), int64(7); got != want {
		t.Fatalf("unexpected keep_daily: got %d, want %d", got, want)
	}
	if !data.VerifyNew.ValueBool() {
		t.Fatal("expected verify_new to be true")
	}
}

func assertStringPointer(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("unexpected %s: got %#v, want %q", name, got, want)
	}
}

func assertInt64Pointer(t *testing.T, name string, got *int64, want int64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("unexpected %s: got %#v, want %d", name, got, want)
	}
}

func assertTruePointer(t *testing.T, name string, got *bool) {
	t.Helper()
	if got == nil || !*got {
		t.Fatalf("unexpected %s: got %#v, want true", name, got)
	}
}
