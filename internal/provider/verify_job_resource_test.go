// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestVerifyJobLifecycle(t *testing.T) {
	ctx := context.Background()
	created, updated, deleted := false, false, false
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api2/json/access/ticket":
			writeDatastoreResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case "/api2/json/config/verify":
			if req.Method == http.MethodGet {
				writeDatastoreResponseWithDigest(w, `[]`, "collection-digest")
				return
			}
			if err := req.ParseForm(); err != nil {
				t.Error(err)
			}
			if req.Method != http.MethodPost || req.Form.Get("id") != "daily" || req.Form.Has("digest") || req.Form.Get("ignore-verified") != "false" || req.Form.Get("max-depth") != "0" {
				t.Errorf("unexpected create request: %s %#v", req.Method, req.Form)
			}
			created = true
			writeDatastoreResponse(w, `null`)
		case "/api2/json/config/verify/daily":
			if req.Method == http.MethodGet {
				reads++
				if deleted {
					http.Error(w, "no such verification 'daily'", http.StatusBadRequest)
					return
				}
				data := `{"id":"daily","store":"backups"}`
				if !updated {
					data = `{"id":"daily","store":"backups","schedule":"09:00","comment":"Daily verification","ns":"child","max-depth":0,"ignore-verified":false,"outdated-after":30,"read-threads":1,"verify-threads":2}`
				}
				writeDatastoreResponseWithDigest(w, data, fmt.Sprintf("digest-%d", reads))
				return
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Error(err)
			}
			req.Form, err = url.ParseQuery(string(body))
			if err != nil {
				t.Error(err)
			}
			if req.Form.Get("digest") != fmt.Sprintf("digest-%d", reads) || req.Form.Has("id") {
				t.Errorf("mutation did not use fresh digest: %#v", req.Form)
			}
			switch req.Method {
			case http.MethodPut:
				want := []string{"schedule", "comment", "ns", "max-depth", "ignore-verified", "outdated-after", "read-threads", "verify-threads"}
				if !reflect.DeepEqual(req.Form["delete"], want) || req.Form.Get("store") != "backups" {
					t.Errorf("unexpected update form: %#v", req.Form)
				}
				updated = true
			case http.MethodDelete:
				if req.Form.Has("delete") || req.Form.Has("store") {
					t.Errorf("unexpected delete form: %#v", req.Form)
				}
				deleted = true
			default:
				t.Errorf("unexpected method: %s", req.Method)
			}
			writeDatastoreResponse(w, `null`)
		default:
			t.Errorf("unexpected API path: %s", req.URL.Path)
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)
	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatal(err)
	}
	r := &VerifyJobResource{client: client}
	data := VerifyJobResourceModel{
		ID: types.StringValue("daily"), Store: types.StringValue("backups"),
		Schedule: types.StringValue("09:00"), Comment: types.StringValue("Daily verification"),
		Namespace: types.StringValue("child"), MaxDepth: types.Int64Value(0),
		IgnoreVerified: types.BoolValue(false), OutdatedAfter: types.Int64Value(30),
		ReadThreads: types.Int64Value(1), VerifyThreads: types.Int64Value(2),
	}
	plan := verifyJobTestPlan(t, data)
	create := resource.CreateResponse{State: tfsdk.State(plan)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &create)
	if create.Diagnostics.HasError() || !created {
		t.Fatalf("create failed: %v", create.Diagnostics)
	}
	var actual VerifyJobResourceModel
	if diags := create.State.Get(ctx, &actual); diags.HasError() || !reflect.DeepEqual(actual, data) {
		t.Fatalf("incorrect created state: %#v; %v", actual, diags)
	}

	minimal := VerifyJobResourceModel{ID: data.ID, Store: data.Store}
	minimalPlan := verifyJobTestPlan(t, minimal)
	update := resource.UpdateResponse{State: create.State}
	r.Update(ctx, resource.UpdateRequest{Plan: minimalPlan, State: create.State}, &update)
	if update.Diagnostics.HasError() || !updated {
		t.Fatalf("update failed: %v", update.Diagnostics)
	}
	if diags := update.State.Get(ctx, &actual); diags.HasError() || !reflect.DeepEqual(actual, minimal) {
		t.Fatalf("optional fields were not cleared: %#v; %v", actual, diags)
	}

	imported := resource.ImportStateResponse{State: tfsdk.State(minimalPlan)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "daily"}, &imported)
	read := resource.ReadResponse{State: imported.State}
	r.Read(ctx, resource.ReadRequest{State: imported.State}, &read)
	if imported.Diagnostics.HasError() || read.Diagnostics.HasError() {
		t.Fatalf("import/read failed: %v %v", imported.Diagnostics, read.Diagnostics)
	}
	remove := resource.DeleteResponse{State: read.State}
	r.Delete(ctx, resource.DeleteRequest{State: read.State}, &remove)
	if remove.Diagnostics.HasError() || !deleted {
		t.Fatalf("delete failed: %v", remove.Diagnostics)
	}
	r.Delete(ctx, resource.DeleteRequest{State: read.State}, &remove)
	if remove.Diagnostics.HasError() {
		t.Fatalf("missing job delete failed: %v", remove.Diagnostics)
	}
	r.Read(ctx, resource.ReadRequest{State: read.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
		t.Fatalf("missing job was not removed from state: %v", read.Diagnostics)
	}
}

func TestVerifyJobCreateRetainsOwnershipOnReadFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api2/json/access/ticket":
			writeDatastoreResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case "/api2/json/config/verify":
			if req.Method == http.MethodGet {
				writeDatastoreResponseWithDigest(w, `[]`, "digest")
			} else {
				writeDatastoreResponse(w, `null`)
			}
		default:
			http.Error(w, "read failed", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatal(err)
	}
	data := VerifyJobResourceModel{ID: types.StringValue("daily"), Store: types.StringValue("backups")}
	plan := verifyJobTestPlan(t, data)
	create := resource.CreateResponse{State: tfsdk.State(plan)}
	r := &VerifyJobResource{client: client}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &create)
	var actual VerifyJobResourceModel
	if !create.Diagnostics.HasError() {
		t.Fatal("expected a read failure")
	}
	if diags := create.State.Get(context.Background(), &actual); diags.HasError() || !reflect.DeepEqual(actual, data) {
		t.Fatalf("lost ownership after successful POST: %#v; %v", actual, diags)
	}
}

func verifyJobTestPlan(t *testing.T, data VerifyJobResourceModel) tfsdk.Plan {
	t.Helper()
	var response resource.SchemaResponse
	NewVerifyJobResource().Schema(context.Background(), resource.SchemaRequest{}, &response)
	plan := tfsdk.Plan{Schema: response.Schema}
	if diags := plan.Set(context.Background(), &data); diags.HasError() {
		t.Fatal(diags)
	}
	return plan
}

func TestVerifyJobRejectsValuesPBSWouldNormalize(t *testing.T) {
	for _, test := range []struct {
		name               string
		comment, namespace types.String
		invalid            bool
	}{
		{name: "omitted"},
		{name: "valid", comment: types.StringValue("Daily verification"), namespace: types.StringValue("child")},
		{name: "unknown", comment: types.StringUnknown(), namespace: types.StringUnknown()},
		{name: "empty comment", comment: types.StringValue(""), invalid: true},
		{name: "trimmed comment", comment: types.StringValue(" comment "), invalid: true},
		{name: "empty namespace", namespace: types.StringValue(""), invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := verifyJobTestPlan(t, VerifyJobResourceModel{ID: types.StringValue("daily"), Store: types.StringValue("backups"), Comment: test.comment, Namespace: test.namespace})
			var response resource.ValidateConfigResponse
			(&VerifyJobResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config(plan)}, &response)
			if response.Diagnostics.HasError() != test.invalid {
				t.Fatalf("unexpected validation result: %v", response.Diagnostics)
			}
		})
	}
}
