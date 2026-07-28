// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUserCreatePersistsRecoveryStateWhenRefreshFails(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeAccessTestResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/access/users" {
				t.Fatalf("unexpected digest read: %s %s", r.Method, r.URL.Path)
			}
			writeAccessTestResponseWithDigest(w, `[]`)
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/api2/json/access/users" {
				t.Fatalf("unexpected create request: %s %s", r.Method, r.URL.Path)
			}
			writeAccessTestResponse(w, `null`)
		case 4:
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
	plan := accessUserPlan(t, UserResourceModel{
		UserID: types.StringValue("alice@pbs"), Enable: types.BoolValue(true),
		Comment: types.StringValue("managed"), Expire: types.Int64Value(0),
	})
	response := resource.CreateResponse{State: tfsdk.State(plan)}
	resourceUnderTest := UserResource{client: client}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected refresh error")
	}

	var recovered UserResourceModel
	if diags := response.State.Get(context.Background(), &recovered); diags.HasError() {
		t.Fatalf("reading recovery state returned diagnostics: %#v", diags)
	}
	if got, want := recovered.UserID.ValueString(), "alice@pbs"; got != want {
		t.Fatalf("unexpected recovered user ID: got %q, want %q", got, want)
	}
	if got, want := recovered.Comment.ValueString(), "managed"; got != want {
		t.Fatalf("unexpected recovered comment: got %q, want %q", got, want)
	}
}

func TestUserUpdateUsesFreshDigestAndRepeatedDeletes(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeAccessTestResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/access/users/alice@pbs" {
				t.Fatalf("unexpected endpoint-specific digest read: %s %s", r.Method, r.URL.Path)
			}
			writeAccessTestResponseWithDigest(w, `{"userid":"alice@pbs"}`)
		case 3:
			if r.Method != http.MethodPut || r.URL.Path != "/api2/json/access/users/alice@pbs" {
				t.Fatalf("unexpected user update: %s %s", r.Method, r.URL.Path)
			}
			var payload map[string]any
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode update payload: %s", err)
			}
			if got, want := payload["digest"], "fresh-digest"; got != want {
				t.Fatalf("unexpected update digest: got %#v, want %q", got, want)
			}
			if got, want := payload["delete"], []any{"comment", "email", "firstname", "lastname"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("unexpected update delete values: got %#v, want %#v", got, want)
			}
			writeAccessTestResponse(w, `null`)
		case 4:
			writeAccessTestResponse(w, `{"userid":"alice@pbs","comment":"","email":"","firstname":"","lastname":"","expire":0}`)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	stateData := UserResourceModel{
		UserID: types.StringValue("alice@pbs"), Enable: types.BoolValue(true),
		Comment: types.StringValue("old comment"), Email: types.StringValue("old@example.com"),
		Firstname: types.StringValue("Alice"), Lastname: types.StringValue("Example"), Expire: types.Int64Value(12),
	}
	planData := stateData
	planData.Comment = types.StringNull()
	planData.Email = types.StringNull()
	planData.Firstname = types.StringNull()
	planData.Lastname = types.StringNull()
	planData.Expire = types.Int64Value(0)
	plan := accessUserPlan(t, planData)
	state := tfsdk.State(plan)
	if diags := state.Set(context.Background(), &stateData); diags.HasError() {
		t.Fatalf("setting user state returned diagnostics: %#v", diags)
	}
	response := resource.UpdateResponse{State: state}
	resourceUnderTest := UserResource{client: client}
	resourceUnderTest.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected update diagnostics: %#v", response.Diagnostics)
	}
}

func TestUserTokenCreatePreservesOneTimeSecretBeforeRefresh(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeAccessTestResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/access/users/alice@pbs/token" {
				t.Fatalf("unexpected token list digest read: %s %s", r.Method, r.URL.Path)
			}
			writeAccessTestResponseWithDigest(w, `[]`)
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/api2/json/access/users/alice@pbs/token/demo" {
				t.Fatalf("unexpected token create: %s %s", r.Method, r.URL.Path)
			}
			var payload map[string]any
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode token create payload: %s", err)
			}
			if got, want := payload["digest"], "fresh-digest"; got != want {
				t.Fatalf("unexpected token create digest: got %#v, want %q", got, want)
			}
			writeAccessTestResponse(w, `{"secret":"one-time-secret"}`)
		case 4:
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
	plan := accessTokenPlan(t, UserTokenResourceModel{
		UserID: types.StringValue("alice@pbs"), TokenName: types.StringValue("demo"),
		Enable: types.BoolValue(true), Expire: types.Int64Value(0), Regenerate: types.BoolValue(false),
	})
	response := resource.CreateResponse{State: tfsdk.State(plan)}
	resourceUnderTest := UserTokenResource{client: client}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected refresh error")
	}
	var recovered UserTokenResourceModel
	if diags := response.State.Get(context.Background(), &recovered); diags.HasError() {
		t.Fatalf("reading token recovery state returned diagnostics: %#v", diags)
	}
	if got, want := recovered.ID.ValueString(), "alice@pbs!demo"; got != want {
		t.Fatalf("unexpected recovered token ID: got %q, want %q", got, want)
	}
	if got, want := recovered.Value.ValueString(), "one-time-secret"; got != want {
		t.Fatalf("unexpected recovered token secret: got %q, want %q", got, want)
	}
}

func TestACLCreateUsesFreshDigestAndPersistsTypedRecoveryState(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeAccessTestResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/access/acl" {
				t.Fatalf("unexpected ACL digest read: %s %s", r.Method, r.URL.Path)
			}
			writeAccessTestResponseWithDigest(w, `[]`)
		case 3:
			if r.Method != http.MethodPut || r.URL.Path != "/api2/json/access/acl" {
				t.Fatalf("unexpected ACL create: %s %s", r.Method, r.URL.Path)
			}
			var payload map[string]any
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode ACL payload: %s", err)
			}
			if got, want := payload["digest"], "fresh-digest"; got != want {
				t.Fatalf("unexpected ACL digest: got %#v, want %q", got, want)
			}
			if got, want := payload["group"], "backup-admins"; got != want {
				t.Fatalf("unexpected ACL group: got %#v, want %q", got, want)
			}
			if got, want := payload["role"], "Audit"; got != want {
				t.Fatalf("unexpected ACL role: got %#v, want %q", got, want)
			}
			for _, responseAlias := range []string{"ugid", "ugid_type", "ugid-type", "roleid"} {
				if _, ok := payload[responseAlias]; ok {
					t.Fatalf("ACL mutation payload unexpectedly used response alias %q", responseAlias)
				}
			}
			writeAccessTestResponse(w, `null`)
		case 4:
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
	NewACLResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	planData := ACLResourceModel{
		Path: types.StringValue("/"), AuthID: types.StringValue("backup-admins"),
		UGIDType: types.StringValue("group"), Role: types.StringValue("Audit"),
		Propagate: types.BoolValue(true),
	}
	if diags := plan.Set(context.Background(), &planData); diags.HasError() {
		t.Fatalf("setting ACL plan returned diagnostics: %#v", diags)
	}
	response := resource.CreateResponse{State: tfsdk.State(plan)}
	resourceUnderTest := ACLResource{client: client}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected ACL refresh error")
	}
	var recovered ACLResourceModel
	if diags := response.State.Get(context.Background(), &recovered); diags.HasError() {
		t.Fatalf("reading ACL recovery state returned diagnostics: %#v", diags)
	}
	if got, want := recovered.ID.ValueString(), "/|group|backup-admins|Audit"; got != want {
		t.Fatalf("unexpected recovered ACL ID: got %q, want %q", got, want)
	}
}

func TestACLResourceUsesHostedUserAndGroupWireContract(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeAccessTestResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			assertACLProtocolRequest(t, r, http.MethodGet)
			writeAccessTestResponseWithDigest(w, `[]`)
		case 3:
			assertACLProtocolRequest(t, r, http.MethodPut)
			payload := decodeAccessJSON(t, r)
			assertAccessField(t, payload, "auth-id", "alice@pbs")
			assertAccessField(t, payload, "role", "Audit")
			assertAccessField(t, payload, "path", "/")
			assertAccessField(t, payload, "digest", "fresh-digest")
			if _, ok := payload["group"]; ok {
				t.Fatal("user ACL mutation unexpectedly contained group")
			}
			assertNoACLResponseAliases(t, payload)
			writeAccessTestResponse(w, `null`)
		case 4:
			writeAccessTestResponse(w, `[{"path":"/","ugid":"alice@pbs","ugid_type":"user","roleid":"Audit","propagate":1}]`)
		case 5:
			assertACLProtocolRequest(t, r, http.MethodGet)
			writeAccessTestResponseWithDigest(w, `[]`)
		case 6:
			assertACLProtocolRequest(t, r, http.MethodPut)
			payload := decodeAccessJSON(t, r)
			assertAccessField(t, payload, "group", "backup-admins")
			assertAccessField(t, payload, "role", "Audit")
			assertAccessField(t, payload, "path", "/")
			assertAccessField(t, payload, "digest", "fresh-digest")
			if _, ok := payload["auth-id"]; ok {
				t.Fatal("group ACL mutation unexpectedly contained auth-id")
			}
			assertNoACLResponseAliases(t, payload)
			writeAccessTestResponse(w, `null`)
		case 7:
			writeAccessTestResponse(w, `[{"path":"/","ugid":"backup-admins","ugid_type":"group","roleid":"Audit","propagate":1}]`)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	userPlan := accessACLPlan(t, ACLResourceModel{
		Path: types.StringValue("/"), AuthID: types.StringValue("alice@pbs"),
		UGIDType: types.StringValue("user"), Role: types.StringValue("Audit"), Propagate: types.BoolValue(true),
	})
	userResponse := resource.CreateResponse{State: tfsdk.State(userPlan)}
	resourceUnderTest := ACLResource{client: client}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: userPlan}, &userResponse)
	if userResponse.Diagnostics.HasError() {
		t.Fatalf("unexpected user ACL diagnostics: %#v", userResponse.Diagnostics)
	}
	var userState ACLResourceModel
	if diags := userResponse.State.Get(context.Background(), &userState); diags.HasError() {
		t.Fatalf("reading user ACL state returned diagnostics: %#v", diags)
	}
	if got, want := userState.AuthID.ValueString(), "alice@pbs"; got != want {
		t.Fatalf("unexpected decoded user ACL auth ID: got %q, want %q", got, want)
	}
	if got, want := userState.Role.ValueString(), "Audit"; got != want {
		t.Fatalf("unexpected decoded user ACL role: got %q, want %q", got, want)
	}

	groupPlan := accessACLPlan(t, ACLResourceModel{
		Path: types.StringValue("/"), AuthID: types.StringValue("backup-admins"),
		UGIDType: types.StringValue("group"), Role: types.StringValue("Audit"), Propagate: types.BoolValue(true),
	})
	groupResponse := resource.CreateResponse{State: tfsdk.State(groupPlan)}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: groupPlan}, &groupResponse)
	if groupResponse.Diagnostics.HasError() {
		t.Fatalf("unexpected group ACL diagnostics: %#v", groupResponse.Diagnostics)
	}
	var groupState ACLResourceModel
	if diags := groupResponse.State.Get(context.Background(), &groupState); diags.HasError() {
		t.Fatalf("reading group ACL state returned diagnostics: %#v", diags)
	}
	if got, want := groupState.UGIDType.ValueString(), "group"; got != want {
		t.Fatalf("unexpected decoded group ACL type: got %q, want %q", got, want)
	}
}

func accessACLPlan(t *testing.T, data ACLResourceModel) tfsdk.Plan {
	t.Helper()
	var schemaResponse resource.SchemaResponse
	NewACLResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	if diags := plan.Set(context.Background(), &data); diags.HasError() {
		t.Fatalf("setting ACL plan returned diagnostics: %#v", diags)
	}
	return plan
}

func assertACLProtocolRequest(t *testing.T, request *http.Request, method string) {
	t.Helper()
	const path = "/api2/json/access/acl"
	if request.Method != method || request.URL.Path != path {
		t.Fatalf("unexpected ACL request: got %s %s, want %s %s", request.Method, request.URL.Path, method, path)
	}
}

func decodeAccessJSON(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		t.Fatalf("decode ACL request JSON: %s", err)
	}
	return payload
}

func assertAccessField(t *testing.T, payload map[string]any, name, want string) {
	t.Helper()
	if got := payload[name]; got != want {
		t.Fatalf("unexpected ACL field %s: got %#v, want %q", name, got, want)
	}
}

func assertNoACLResponseAliases(t *testing.T, payload map[string]any) {
	t.Helper()
	for _, name := range []string{"ugid", "ugid_type", "ugid-type", "roleid"} {
		if _, ok := payload[name]; ok {
			t.Fatalf("ACL mutation unexpectedly contained response alias %q", name)
		}
	}
}

func TestUserTokenRegenerationPersistsSecretBeforeRefreshFailure(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeAccessTestResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/access/users/alice@pbs/token/demo" {
				t.Fatalf("unexpected token digest read: %s %s", r.Method, r.URL.Path)
			}
			writeAccessTestResponseWithDigest(w, `{"tokenid":"demo"}`)
		case 3:
			if r.Method != http.MethodPut || r.URL.Path != "/api2/json/access/users/alice@pbs/token/demo" {
				t.Fatalf("unexpected token regeneration: %s %s", r.Method, r.URL.Path)
			}
			payload := decodeAccessJSON(t, r)
			assertAccessField(t, payload, "digest", "fresh-digest")
			if got, ok := payload["regenerate"]; !ok || got != true {
				t.Fatalf("regeneration request omitted regenerate=true: %#v", payload)
			}
			writeAccessTestResponse(w, `{"secret":"rotated-secret"}`)
		case 4:
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
	stateData := UserTokenResourceModel{
		ID: types.StringValue("alice@pbs!demo"), UserID: types.StringValue("alice@pbs"), TokenName: types.StringValue("demo"),
		Enable: types.BoolValue(true), Comment: types.StringValue("managed"), Expire: types.Int64Value(0),
		Regenerate: types.BoolValue(false), Value: types.StringValue("old-secret"),
	}
	planData := stateData
	planData.Regenerate = types.BoolValue(true)
	plan := accessTokenPlan(t, planData)
	state := tfsdk.State(plan)
	if diags := state.Set(context.Background(), &stateData); diags.HasError() {
		t.Fatalf("setting token state returned diagnostics: %#v", diags)
	}
	response := resource.UpdateResponse{State: state}
	resourceUnderTest := UserTokenResource{client: client}
	resourceUnderTest.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: state}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected token refresh error")
	}
	var recovered UserTokenResourceModel
	if diags := response.State.Get(context.Background(), &recovered); diags.HasError() {
		t.Fatalf("reading token regeneration recovery state returned diagnostics: %#v", diags)
	}
	if got, want := recovered.Value.ValueString(), "rotated-secret"; got != want {
		t.Fatalf("unexpected recovered rotated secret: got %q, want %q", got, want)
	}
}

func TestUserTokenMetadataUpdateDoesNotRepeatRegeneration(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeAccessTestResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/access/users/alice@pbs/token/demo" {
				t.Fatalf("unexpected token digest read: %s %s", r.Method, r.URL.Path)
			}
			writeAccessTestResponseWithDigest(w, `{"tokenid":"demo"}`)
		case 3:
			if r.Method != http.MethodPut || r.URL.Path != "/api2/json/access/users/alice@pbs/token/demo" {
				t.Fatalf("unexpected metadata update: %s %s", r.Method, r.URL.Path)
			}
			payload := decodeAccessJSON(t, r)
			assertAccessField(t, payload, "digest", "fresh-digest")
			assertAccessField(t, payload, "comment", "updated")
			if _, ok := payload["regenerate"]; ok {
				t.Fatalf("metadata update unexpectedly repeated regeneration: %#v", payload)
			}
			writeAccessTestResponse(w, `null`)
		case 4:
			writeAccessTestResponse(w, `{"tokenid":"demo","comment":"updated","expire":0}`)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	stateData := UserTokenResourceModel{
		ID: types.StringValue("alice@pbs!demo"), UserID: types.StringValue("alice@pbs"), TokenName: types.StringValue("demo"),
		Enable: types.BoolValue(true), Comment: types.StringValue("old"), Expire: types.Int64Value(0),
		Regenerate: types.BoolValue(true), Value: types.StringValue("rotated-secret"),
	}
	planData := stateData
	planData.Comment = types.StringValue("updated")
	plan := accessTokenPlan(t, planData)
	state := tfsdk.State(plan)
	if diags := state.Set(context.Background(), &stateData); diags.HasError() {
		t.Fatalf("setting token state returned diagnostics: %#v", diags)
	}
	response := resource.UpdateResponse{State: state}
	resourceUnderTest := UserTokenResource{client: client}
	resourceUnderTest.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected metadata update diagnostics: %#v", response.Diagnostics)
	}
	var updated UserTokenResourceModel
	if diags := response.State.Get(context.Background(), &updated); diags.HasError() {
		t.Fatalf("reading metadata update state returned diagnostics: %#v", diags)
	}
	if got, want := updated.Value.ValueString(), "rotated-secret"; got != want {
		t.Fatalf("metadata update did not preserve token secret: got %q, want %q", got, want)
	}
}

func accessUserPlan(t *testing.T, data UserResourceModel) tfsdk.Plan {
	t.Helper()
	var schemaResponse resource.SchemaResponse
	NewUserResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	if diags := plan.Set(context.Background(), &data); diags.HasError() {
		t.Fatalf("setting user plan returned diagnostics: %#v", diags)
	}
	return plan
}

func accessTokenPlan(t *testing.T, data UserTokenResourceModel) tfsdk.Plan {
	t.Helper()
	var schemaResponse resource.SchemaResponse
	NewUserTokenResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	if diags := plan.Set(context.Background(), &data); diags.HasError() {
		t.Fatalf("setting token plan returned diagnostics: %#v", diags)
	}
	return plan
}

func writeAccessTestResponse(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"data":%s}`, data)
}

func writeAccessTestResponseWithDigest(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"data":%s,"digest":"fresh-digest"}`, data)
}
