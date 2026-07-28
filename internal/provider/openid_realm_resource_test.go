// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestOpenIDRealmResourceUsesPBSFormMutationsAndPreservesClientKey(t *testing.T) {
	const configuredClientKey = "configured-client-key"
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		if requestNumber == 1 {
			if r.URL.Path != "/api2/json/access/ticket" || r.Method != http.MethodPost {
				t.Fatalf("unexpected authentication request: %s %s", r.Method, r.URL.Path)
			}
			if got, want := r.FormValue("username"), "root@pam"; got != want {
				t.Fatalf("unexpected authentication username: got %q, want %q", got, want)
			}
			if got, want := r.FormValue("password"), "password"; got != want {
				t.Fatalf("unexpected authentication password: got %q, want %q", got, want)
			}
			writeOpenIDResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
			return
		}

		if r.Method != http.MethodGet {
			if got, want := r.Header.Get("CSRFPreventionToken"), "csrf"; got != want {
				t.Fatalf("unexpected CSRF token: got %q, want %q", got, want)
			}
		}
		if _, err := r.Cookie("PBSAuthCookie"); err != nil {
			t.Fatalf("missing authentication cookie: %s", err)
		}

		switch requestNumber {
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/api2/json/config/access/openid" {
				t.Fatalf("unexpected create request: %s %s", r.Method, r.URL.Path)
			}
			assertOpenIDFormValue(t, r, "realm", "openid")
			assertOpenIDFormValue(t, r, "issuer-url", "https://issuer.example.com")
			assertOpenIDFormValue(t, r, "client-id", "client-id")
			assertOpenIDFormValue(t, r, "client-key", configuredClientKey)
			assertOpenIDFormValue(t, r, "scopes", defaultOpenIDRealmScopes)
			assertOpenIDFormValue(t, r, "autocreate", "true")
			assertOpenIDFormValue(t, r, "username-claim", "preferred_username")
			if got := r.Form["delete"]; len(got) != 0 {
				t.Fatalf("create request unexpectedly contained delete parameters: %#v", got)
			}
			writeOpenIDResponse(w, `null`)
		case 3:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/access/openid/openid" {
				t.Fatalf("unexpected first read request: %s %s", r.Method, r.URL.Path)
			}
			// The API value is intentionally different: resource state must retain
			// the configured value and must not persist a read response secret.
			writeOpenIDResponseWithDigest(w, "digest-1", `{"realm":"openid","issuer-url":"https://issuer.example.com","client-id":"client-id","client-key":"api-redaction","scopes":"email profile","autocreate":true,"acr-values":"urn:example","prompt":"login","comment":"managed","username-claim":"preferred_username"}`)
		case 4:
			if r.Method != http.MethodPut || r.URL.Path != "/api2/json/config/access/openid/openid" {
				t.Fatalf("unexpected update request: %s %s", r.Method, r.URL.Path)
			}
			if got, want := r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"; got != want {
				t.Fatalf("unexpected update content type: got %q, want %q", got, want)
			}
			assertOpenIDFormValue(t, r, "digest", "digest-1")
			assertOpenIDFormValue(t, r, "issuer-url", "https://new-issuer.example.com")
			assertOpenIDFormValue(t, r, "client-id", "new-client-id")
			assertOpenIDFormValue(t, r, "autocreate", "false")
			if got := r.FormValue("client-key"); got != "" {
				t.Fatalf("cleared client key was sent in update form: %q", got)
			}
			if got, want := r.Form["delete"], []string{"client-key", "acr-values", "prompt", "comment"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("unexpected repeated delete parameters: got %#v, want %#v", got, want)
			}
			writeOpenIDResponse(w, `null`)
		case 5:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/access/openid/openid" {
				t.Fatalf("unexpected second read request: %s %s", r.Method, r.URL.Path)
			}
			writeOpenIDResponseWithDigest(w, "digest-2", `{"realm":"openid","issuer-url":"https://new-issuer.example.com","client-id":"new-client-id","scopes":"email profile","autocreate":false}`)
		case 6:
			if r.Method != http.MethodDelete || r.URL.Path != "/api2/json/config/access/openid/openid" {
				t.Fatalf("unexpected delete request: %s %s", r.Method, r.URL.Path)
			}
			if got, want := r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"; got != want {
				t.Fatalf("unexpected delete content type: got %q, want %q", got, want)
			}
			assertOpenIDDeleteFormValue(t, r, "digest", "digest-2")
			writeOpenIDResponse(w, `null`)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	data := OpenIDRealmResourceModel{
		Realm:         types.StringValue("openid"),
		IssuerURL:     types.StringValue("https://issuer.example.com"),
		ClientID:      types.StringValue("client-id"),
		ClientKey:     types.StringValue(configuredClientKey),
		Scopes:        types.StringValue(defaultOpenIDRealmScopes),
		AutoCreate:    types.BoolValue(true),
		UsernameClaim: types.StringValue("preferred_username"),
	}
	resource := OpenIDRealmResource{client: client}
	if err := client.postForm(context.Background(), "/config/access/openid", openIDRealmForm(data, true), nil); err != nil {
		t.Fatalf("create returned error: %s", err)
	}
	if err := resource.readOpenIDRealm(context.Background(), &data); err != nil {
		t.Fatalf("first read returned error: %s", err)
	}
	if got, want := data.Digest.ValueString(), "digest-1"; got != want {
		t.Fatalf("unexpected first digest: got %q, want %q", got, want)
	}
	if got, want := data.ClientKey.ValueString(), configuredClientKey; got != want {
		t.Fatalf("client key was not preserved: got %q, want %q", got, want)
	}

	plan := data
	plan.IssuerURL = types.StringValue("https://new-issuer.example.com")
	plan.ClientID = types.StringValue("new-client-id")
	plan.ClientKey = types.StringNull()
	plan.ACRValues = types.StringNull()
	plan.Prompt = types.StringNull()
	plan.Comment = types.StringNull()
	plan.AutoCreate = types.BoolValue(false)
	updateForm := openIDRealmForm(plan, false)
	for _, field := range openIDRealmDeletedFields(plan, data) {
		updateForm.Add("delete", field)
	}
	setOpenIDDigest(updateForm, data.Digest)
	if err := client.putForm(context.Background(), "/config/access/openid/openid", updateForm); err != nil {
		t.Fatalf("update returned error: %s", err)
	}
	if err := resource.readOpenIDRealm(context.Background(), &plan); err != nil {
		t.Fatalf("second read returned error: %s", err)
	}
	if !plan.ClientKey.IsNull() {
		t.Fatal("cleared client key was retained after update")
	}
	if !plan.ACRValues.IsNull() || !plan.Prompt.IsNull() || !plan.Comment.IsNull() {
		t.Fatal("absent optional API fields were not represented as null")
	}
	if got, want := plan.ID.ValueString(), "openid"; got != want {
		t.Fatalf("unexpected resource ID: got %q, want %q", got, want)
	}
	if got, want := plan.Digest.ValueString(), "digest-2"; got != want {
		t.Fatalf("unexpected refreshed digest: got %q, want %q", got, want)
	}
	deleteForm := url.Values{}
	setOpenIDDigest(deleteForm, plan.Digest)
	if err := client.deleteForm(context.Background(), "/config/access/openid/openid", deleteForm); err != nil {
		t.Fatalf("delete returned error: %s", err)
	}
}

func TestOpenIDRealmDeletedFieldsRepeatsPBSDeleteParameter(t *testing.T) {
	plan := OpenIDRealmResourceModel{
		ClientKey:  types.StringNull(),
		Scopes:     types.StringNull(),
		ACRValues:  types.StringNull(),
		Prompt:     types.StringNull(),
		Comment:    types.StringNull(),
		AutoCreate: types.BoolValue(false),
	}
	state := OpenIDRealmResourceModel{
		ClientKey:  types.StringValue("not-used"),
		Scopes:     types.StringValue("email profile"),
		ACRValues:  types.StringValue("acr"),
		Prompt:     types.StringValue("login"),
		Comment:    types.StringValue("comment"),
		AutoCreate: types.BoolValue(true),
	}

	form := openIDRealmForm(plan, false)
	for _, field := range openIDRealmDeletedFields(plan, state) {
		form.Add("delete", field)
	}
	if got, want := form["delete"], []string{"client-key", "scopes", "acr-values", "prompt", "comment"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected delete form values: got %#v, want %#v", got, want)
	}
}

func TestOpenIDRealmRecoveryStateClearsUnknownComputedValues(t *testing.T) {
	data := OpenIDRealmResourceModel{
		Realm:         types.StringValue("openid"),
		IssuerURL:     types.StringValue("https://issuer.example.com"),
		ClientID:      types.StringValue("client-id"),
		Scopes:        types.StringUnknown(),
		AutoCreate:    types.BoolUnknown(),
		UsernameClaim: types.StringUnknown(),
		ID:            types.StringUnknown(),
		Digest:        types.StringUnknown(),
	}

	recovery := openIDRealmRecoveryState(data)
	if got, want := recovery.ID.ValueString(), "openid"; got != want {
		t.Fatalf("unexpected recovery ID: got %q, want %q", got, want)
	}
	if !recovery.Digest.IsNull() || !recovery.Scopes.IsNull() || !recovery.AutoCreate.IsNull() || !recovery.UsernameClaim.IsNull() {
		t.Fatalf("unknown computed values were not cleared: %#v", recovery)
	}
	if got, want := recovery.IssuerURL.ValueString(), "https://issuer.example.com"; got != want {
		t.Fatalf("known planned value was not preserved: got %q, want %q", got, want)
	}
}

func TestOpenIDRealmCreatePersistsRecoveryStateWhenRefreshFails(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeOpenIDResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/api2/json/config/access/openid" {
				t.Fatalf("unexpected create request: %s %s", r.Method, r.URL.Path)
			}
			writeOpenIDResponse(w, `null`)
		case 3:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/access/openid/openid" {
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

	var schemaResp resource.SchemaResponse
	NewOpenIDRealmResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	planData := OpenIDRealmResourceModel{
		Realm:         types.StringValue("openid"),
		IssuerURL:     types.StringValue("https://issuer.example.com"),
		ClientID:      types.StringValue("client-id"),
		ClientKey:     types.StringNull(),
		Scopes:        types.StringValue(defaultOpenIDRealmScopes),
		ACRValues:     types.StringNull(),
		Prompt:        types.StringNull(),
		Comment:       types.StringNull(),
		AutoCreate:    types.BoolValue(false),
		UsernameClaim: types.StringUnknown(),
		ID:            types.StringUnknown(),
		Digest:        types.StringUnknown(),
	}
	if diags := plan.Set(context.Background(), &planData); diags.HasError() {
		t.Fatalf("setting test plan returned diagnostics: %#v", diags)
	}

	response := resource.CreateResponse{State: tfsdk.State(plan)}
	resourceUnderTest := OpenIDRealmResource{client: client}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected refresh error")
	}
	if got, want := requestNumber, 3; got != want {
		t.Fatalf("unexpected request count: got %d, want %d", got, want)
	}

	var recovered OpenIDRealmResourceModel
	if diags := response.State.Get(context.Background(), &recovered); diags.HasError() {
		t.Fatalf("reading recovery state returned diagnostics: %#v", diags)
	}
	if got, want := recovered.Realm.ValueString(), "openid"; got != want {
		t.Fatalf("unexpected recovered realm: got %q, want %q", got, want)
	}
	if got, want := recovered.ID.ValueString(), "openid"; got != want {
		t.Fatalf("unexpected recovered ID: got %q, want %q", got, want)
	}
	if !recovered.Digest.IsNull() || !recovered.UsernameClaim.IsNull() {
		t.Fatalf("recovery state retained unknown computed values: %#v", recovered)
	}
	if got, want := recovered.IssuerURL.ValueString(), "https://issuer.example.com"; got != want {
		t.Fatalf("unexpected recovered issuer URL: got %q, want %q", got, want)
	}
}

func TestOpenIDRealmSchemaProtectsSecretsAndReplacesImmutableAttributes(t *testing.T) {
	var resp resource.SchemaResponse
	NewOpenIDRealmResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)

	clientKey, ok := resp.Schema.Attributes["client_key"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("client_key has type %T, want schema.StringAttribute", resp.Schema.Attributes["client_key"])
	}
	if !clientKey.Sensitive {
		t.Fatal("client_key is not sensitive")
	}
	for _, name := range []string{"realm", "username_claim"} {
		attribute, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s has type %T, want schema.StringAttribute", name, resp.Schema.Attributes[name])
		}
		if len(attribute.PlanModifiers) == 0 {
			t.Fatalf("%s does not have a replace plan modifier", name)
		}
	}
	id, ok := resp.Schema.Attributes["id"].(schema.StringAttribute)
	if !ok || len(id.PlanModifiers) == 0 || !id.Computed {
		t.Fatal("id is not a computed attribute with a state plan modifier")
	}
	digest, ok := resp.Schema.Attributes["digest"].(schema.StringAttribute)
	if !ok || !digest.Computed || digest.Optional || digest.Required {
		t.Fatal("digest is not read-only and computed")
	}
}

func TestOpenIDStringValuePreservesAPIPresentEmptyStrings(t *testing.T) {
	empty := ""
	if got := openIDStringValue(&empty); got.IsNull() || got.ValueString() != "" {
		t.Fatalf("API-present empty string was not preserved: %#v", got)
	}
	if got := openIDStringValue(nil); !got.IsNull() {
		t.Fatal("missing API string was not represented as null")
	}
}

func assertOpenIDFormValue(t *testing.T, r *http.Request, name, want string) {
	t.Helper()
	if got := r.FormValue(name); got != want {
		t.Fatalf("unexpected form value %s: got %q, want %q", name, got, want)
	}
}

func assertOpenIDDeleteFormValue(t *testing.T, r *http.Request, name, want string) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read delete form: %s", err)
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("parse delete form: %s", err)
	}
	if got := form.Get(name); got != want {
		t.Fatalf("unexpected delete form value %s: got %q, want %q", name, got, want)
	}
}

func writeOpenIDResponse(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":` + data + `}`))
}

func writeOpenIDResponseWithDigest(w http.ResponseWriter, digest, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":` + data + `,"digest":` + strconv.Quote(digest) + `}`))
}

func TestOpenIDRealmErrorRedactsClientKey(t *testing.T) {
	err := errors.New("server rejected configured-client-key")
	if got := openIDRealmError(err, types.StringValue("configured-client-key")); strings.Contains(got, "configured-client-key") {
		t.Fatalf("client key was exposed in error: %q", got)
	}
}

func TestOpenIDRealmErrorRedactsJSONEscapedClientKey(t *testing.T) {
	secret := `client"key\path`
	escaped, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %s", err)
	}

	errorBody := `{"errors":{"client-key":` + string(escaped) + `}}`
	got := openIDRealmError(errors.New(errorBody), types.StringValue(secret))
	if strings.Contains(got, string(escaped)) || strings.Contains(got, secret) {
		t.Fatalf("JSON-escaped client key was exposed in error: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redacted error lost useful context: %q", got)
	}
}
