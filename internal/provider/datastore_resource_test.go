// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestDatastoreComputedFieldsDoNotReplaceUnconfiguredBackends(t *testing.T) {
	ctx := context.Background()
	var resourceSchema resource.SchemaResponse
	(&DatastoreResource{}).Schema(ctx, resource.SchemaRequest{}, &resourceSchema)
	existing := tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})
	cases := []struct {
		name                string
		config, state, plan types.String
		replace             bool
	}{
		{"omitted and absent", types.StringNull(), types.StringNull(), types.StringUnknown(), false},
		{"omitted and discovered", types.StringNull(), types.StringValue("old"), types.StringUnknown(), false},
		{"unchanged", types.StringValue("old"), types.StringValue("old"), types.StringValue("old"), false},
		{"changed", types.StringValue("new"), types.StringValue("old"), types.StringValue("new"), true},
		{"newly configured", types.StringValue("new"), types.StringNull(), types.StringValue("new"), true},
		{"configured unknown", types.StringUnknown(), types.StringValue("old"), types.StringUnknown(), true},
	}
	for _, field := range []string{"backend", "backing_device"} {
		attribute, ok := resourceSchema.Schema.Attributes[field].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s is not a string attribute", field)
		}
		for _, tc := range cases {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				req := planmodifier.StringRequest{
					ConfigValue: tc.config, StateValue: tc.state, PlanValue: tc.plan,
					State: tfsdk.State{Raw: existing}, Plan: tfsdk.Plan{Raw: existing},
				}
				replace := false
				for _, modifier := range attribute.PlanModifiers {
					resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
					modifier.PlanModifyString(ctx, req, &resp)
					if resp.Diagnostics.HasError() {
						t.Fatal(resp.Diagnostics)
					}
					req.PlanValue = resp.PlanValue
					replace = replace || resp.RequiresReplace
				}
				if replace != tc.replace {
					t.Fatalf("replacement = %t, want %t", replace, tc.replace)
				}
			})
		}
	}
}

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

func TestDatastoreCreatePersistsRecoveryStateWhenRefreshFails(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeDatastoreResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/datastore" {
				t.Fatalf("unexpected digest request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponseWithDigest(w, `[]`, "fresh-digest")
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/api2/json/config/datastore" {
				t.Fatalf("unexpected create request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponse(w, `null`)
		case 4:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/datastore/backup" {
				t.Fatalf("unexpected refresh request: %s %s", r.Method, r.URL.Path)
			}
			http.Error(w, "refresh failed", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	var schemaResponse resource.SchemaResponse
	NewDatastoreResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	planData := DatastoreResourceModel{
		Name: types.StringValue("backup"),
		Path: types.StringValue("/mnt/datastore/backup"),
	}
	if diags := plan.Set(context.Background(), &planData); diags.HasError() {
		t.Fatalf("setting test plan returned diagnostics: %#v", diags)
	}

	response := resource.CreateResponse{State: tfsdk.State(plan)}
	resourceUnderTest := DatastoreResource{client: client}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected refresh error")
	}

	var recovered DatastoreResourceModel
	if diags := response.State.Get(context.Background(), &recovered); diags.HasError() {
		t.Fatalf("reading recovery state returned diagnostics: %#v", diags)
	}
	if got, want := recovered.Name.ValueString(), "backup"; got != want {
		t.Fatalf("unexpected recovered name: got %q, want %q", got, want)
	}
	if got, want := recovered.Path.ValueString(), "/mnt/datastore/backup"; got != want {
		t.Fatalf("unexpected recovered path: got %q, want %q", got, want)
	}
}

func TestDatastoreCreatePersistsRecoveryStateBeforeUPIDTaskWait(t *testing.T) {
	requestNumber := 0
	upid := "UPID:node:00000001:00000001:00000001:create:backup:root@pam:"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeDatastoreResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/datastore" {
				t.Fatalf("unexpected digest request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponseWithDigest(w, `[]`, "fresh-digest")
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/api2/json/config/datastore" {
				t.Fatalf("unexpected create request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponse(w, fmt.Sprintf(`%q`, upid))
		case 4:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/nodes/node/tasks/UPID:node:00000001:00000001:00000001:create:backup:root@pam:/status" {
				t.Fatalf("unexpected task status request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponse(w, `{"status":"stopped","exitstatus":"ERROR"}`)
		case 5:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/nodes/node/tasks/UPID:node:00000001:00000001:00000001:create:backup:root@pam:/log" {
				t.Fatalf("unexpected task log request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponse(w, `[{"t":"create failed"}]`)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	plan := datastoreTestPlan(t, DatastoreResourceModel{
		Name: types.StringValue("backup"),
		Path: types.StringValue("/mnt/datastore/backup"),
	})
	response := resource.CreateResponse{State: tfsdk.State(plan)}
	resourceUnderTest := DatastoreResource{client: client}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected task failure diagnostics")
	}
	if got := response.Diagnostics[0].Detail(); got == "" || !containsAll(got, "create failed", "ERROR") {
		t.Fatalf("task failure diagnostic did not include task details: %q", got)
	}

	var recovered DatastoreResourceModel
	if diags := response.State.Get(context.Background(), &recovered); diags.HasError() {
		t.Fatalf("reading recovery state returned diagnostics: %#v", diags)
	}
	if got, want := recovered.Name.ValueString(), "backup"; got != want {
		t.Fatalf("unexpected recovered name: got %q, want %q", got, want)
	}
	if got, want := recovered.Path.ValueString(), "/mnt/datastore/backup"; got != want {
		t.Fatalf("unexpected recovered path: got %q, want %q", got, want)
	}
}

func TestDatastoreDeleteAcceptsNullResponseAfterFreshDigestRead(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeDatastoreResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/datastore/backup" {
				t.Fatalf("unexpected digest request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponseWithDigest(w, `{"name":"backup","path":"/mnt/datastore/backup"}`, "fresh-digest")
		case 3:
			if r.Method != http.MethodDelete || r.URL.Path != "/api2/json/config/datastore/backup" {
				t.Fatalf("unexpected delete request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponse(w, `null`)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	state := datastoreTestState(t, DatastoreResourceModel{
		Name: types.StringValue("backup"),
		Path: types.StringValue("/mnt/datastore/backup"),
	})
	response := resource.DeleteResponse{}
	resourceUnderTest := DatastoreResource{client: client}
	resourceUnderTest.Delete(context.Background(), resource.DeleteRequest{State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected delete diagnostics: %#v", response.Diagnostics)
	}
	if got, want := requestNumber, 3; got != want {
		t.Fatalf("unexpected request count: got %d, want %d", got, want)
	}
}

func TestDatastoreDeleteWaitsForUPIDAndReportsTaskFailure(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeDatastoreResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			writeDatastoreResponseWithDigest(w, `{"name":"backup","path":"/mnt/datastore/backup"}`, "fresh-digest")
		case 3:
			if r.Method != http.MethodDelete || r.URL.Path != "/api2/json/config/datastore/backup" {
				t.Fatalf("unexpected delete request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponse(w, `"UPID:node:00000001:00000001:00000001:destroy:backup:root@pam:"`)
		case 4:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/nodes/node/tasks/UPID:node:00000001:00000001:00000001:destroy:backup:root@pam:/status" {
				t.Fatalf("unexpected task status request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponse(w, `{"status":"stopped","exitstatus":"ERROR"}`)
		case 5:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/nodes/node/tasks/UPID:node:00000001:00000001:00000001:destroy:backup:root@pam:/log" {
				t.Fatalf("unexpected task log request: %s %s", r.Method, r.URL.Path)
			}
			writeDatastoreResponse(w, `[{"t":"destroy failed"}]`)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	state := datastoreTestState(t, DatastoreResourceModel{
		Name: types.StringValue("backup"),
		Path: types.StringValue("/mnt/datastore/backup"),
	})
	response := resource.DeleteResponse{}
	resourceUnderTest := DatastoreResource{client: client}
	resourceUnderTest.Delete(context.Background(), resource.DeleteRequest{State: state}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected task failure diagnostics")
	}
	if got := response.Diagnostics[0].Detail(); got == "" || !containsAll(got, "destroy failed", "ERROR") {
		t.Fatalf("task failure diagnostic did not include task details: %q", got)
	}
}

func TestDatastoreUpdateUsesFreshDigest(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeDatastoreResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			writeDatastoreResponseWithDigest(w, `{"name":"backup","path":"/mnt/datastore/backup"}`, "fresh-digest")
		case 3:
			if r.Method != http.MethodPut || r.URL.Path != "/api2/json/config/datastore/backup" {
				t.Fatalf("unexpected update request: %s %s", r.Method, r.URL.Path)
			}
			if got := r.FormValue("digest"); got != "fresh-digest" {
				t.Fatalf("unexpected update digest: got %q", got)
			}
			writeDatastoreResponse(w, `null`)
		case 4:
			writeDatastoreResponse(w, `{"name":"backup","path":"/mnt/datastore/backup","comment":"updated"}`)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	var schemaResponse resource.SchemaResponse
	NewDatastoreResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	planData := DatastoreResourceModel{
		Name:    types.StringValue("backup"),
		Path:    types.StringValue("/mnt/datastore/backup"),
		Comment: types.StringValue("updated"),
	}
	if diags := plan.Set(context.Background(), &planData); diags.HasError() {
		t.Fatalf("setting test plan returned diagnostics: %#v", diags)
	}
	state := datastoreTestState(t, DatastoreResourceModel{
		Name: types.StringValue("backup"),
		Path: types.StringValue("/mnt/datastore/backup"),
	})
	response := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResponse.Schema}}
	resourceUnderTest := DatastoreResource{client: client}
	resourceUnderTest.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected update diagnostics: %#v", response.Diagnostics)
	}
}

func datastoreTestState(t *testing.T, data DatastoreResourceModel) tfsdk.State {
	t.Helper()
	var schemaResponse resource.SchemaResponse
	NewDatastoreResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	state := tfsdk.State{Schema: schemaResponse.Schema}
	if diags := state.Set(context.Background(), &data); diags.HasError() {
		t.Fatalf("setting test state returned diagnostics: %#v", diags)
	}
	return state
}

func datastoreTestPlan(t *testing.T, data DatastoreResourceModel) tfsdk.Plan {
	t.Helper()
	var schemaResponse resource.SchemaResponse
	NewDatastoreResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	if diags := plan.Set(context.Background(), &data); diags.HasError() {
		t.Fatalf("setting test plan returned diagnostics: %#v", diags)
	}
	return plan
}

func writeDatastoreResponse(w http.ResponseWriter, data string) {
	writeDatastoreResponseWithDigest(w, data, "")
}

func writeDatastoreResponseWithDigest(w http.ResponseWriter, data, digest string) {
	w.Header().Set("Content-Type", "application/json")
	if digest == "" {
		_, _ = fmt.Fprintf(w, `{"data":%s}`, data)
		return
	}
	_, _ = fmt.Fprintf(w, `{"data":%s,"digest":%q}`, data, digest)
}

func containsAll(value string, required ...string) bool {
	for _, item := range required {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
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
