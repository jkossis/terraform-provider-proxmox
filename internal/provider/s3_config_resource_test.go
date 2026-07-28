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
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestS3ConfigDeletedFields(t *testing.T) {
	plan := S3ConfigResourceModel{
		Port:           types.Int64Null(),
		Region:         types.StringNull(),
		Fingerprint:    types.StringNull(),
		RateIn:         types.StringNull(),
		BurstIn:        types.StringNull(),
		RateOut:        types.StringNull(),
		BurstOut:       types.StringNull(),
		ProviderQuirks: types.ListNull(types.StringType),
		PutRateLimit:   types.Int64Null(),
	}
	state := S3ConfigResourceModel{
		Port:           types.Int64Value(443),
		Region:         types.StringValue("us-east-1"),
		Fingerprint:    types.StringValue("aa"),
		RateIn:         types.StringValue("1MiB"),
		BurstIn:        types.StringValue("2MiB"),
		RateOut:        types.StringValue("3MiB"),
		BurstOut:       types.StringValue("4MiB"),
		ProviderQuirks: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("skip-if-none-match-header")}),
		PutRateLimit:   types.Int64Value(100),
	}

	got := s3ConfigDeletedFields(plan, state)
	want := []string{"port", "region", "fingerprint", "rate-in", "burst-in", "rate-out", "burst-out", "provider-quirks"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deleted fields: got %#v, want %#v", got, want)
	}
}

func TestS3ConfigDeletedFieldsDeletesProviderQuirksWhenPlanIsEmpty(t *testing.T) {
	plan := S3ConfigResourceModel{
		ProviderQuirks: types.ListValueMust(types.StringType, []attr.Value{}),
	}
	state := S3ConfigResourceModel{
		ProviderQuirks: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("skip-if-none-match-header")}),
	}

	got := s3ConfigDeletedFields(plan, state)
	want := []string{"provider-quirks"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected deleted fields: got %#v, want %#v", got, want)
	}
}

func TestS3ConfigPathStyleDeletionDistinguishesOmittedAndExplicitFalse(t *testing.T) {
	state := S3ConfigResourceModel{PathStyle: types.BoolValue(true)}

	omitted := S3ConfigResourceModel{PathStyle: types.BoolNull()}
	if got, want := s3ConfigDeletedFields(omitted, state), []string{"path-style"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected omitted path_style deletions: got %#v, want %#v", got, want)
	}
	omittedForm, diags := s3ConfigForm(context.Background(), omitted)
	if diags.HasError() {
		t.Fatalf("building omitted path_style form returned diagnostics: %#v", diags)
	}
	if _, ok := omittedForm["path-style"]; ok {
		t.Fatal("omitted path_style was sent as an implicit false")
	}

	explicitFalse := S3ConfigResourceModel{PathStyle: types.BoolValue(false)}
	if got := s3ConfigDeletedFields(explicitFalse, state); len(got) != 0 {
		t.Fatalf("unexpected explicit false path_style deletions: %#v", got)
	}
	explicitFalseForm, diags := s3ConfigForm(context.Background(), explicitFalse)
	if diags.HasError() {
		t.Fatalf("building explicit false path_style form returned diagnostics: %#v", diags)
	}
	if got, want := explicitFalseForm.Get("path-style"), "false"; got != want {
		t.Fatalf("unexpected explicit false path_style form value: got %q, want %q", got, want)
	}
}

func TestS3ConfigUpdateUsesRawConfigForPathStyleDeletion(t *testing.T) {
	for _, test := range []struct {
		name            string
		configPathStyle types.Bool
		wantPathStyle   string
		wantDeleteStyle bool
	}{
		{name: "omitted", configPathStyle: types.BoolNull(), wantDeleteStyle: true},
		{name: "explicit false", configPathStyle: types.BoolValue(false), wantPathStyle: "false"},
	} {
		t.Run(test.name, func(t *testing.T) {
			requestNumber := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestNumber++
				switch requestNumber {
				case 1:
					writeS3ConfigResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
				case 2:
					writeS3ConfigResponseWithDigest(w, `{"id":"garage","access-key":"access","endpoint":"garage","path-style":true}`, "fresh-digest")
				case 3:
					if r.Method != http.MethodPut || r.URL.Path != "/api2/json/config/s3/garage" {
						t.Fatalf("unexpected update request: %s %s", r.Method, r.URL.Path)
					}
					if got := r.FormValue("digest"); got != "fresh-digest" {
						t.Fatalf("unexpected update digest: got %q", got)
					}
					if got := r.FormValue("path-style"); got != test.wantPathStyle {
						t.Fatalf("unexpected path_style form value: got %q, want %q", got, test.wantPathStyle)
					}
					if got := r.Form["delete"]; (len(got) == 1 && got[0] == "path-style") != test.wantDeleteStyle {
						t.Fatalf("unexpected path_style delete values: %#v", got)
					}
					writeS3ConfigResponse(w, `null`)
				case 4:
					writeS3ConfigResponse(w, `{"id":"garage","access-key":"access","endpoint":"garage","path-style":false}`)
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
			NewS3ConfigResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
			base := S3ConfigResourceModel{
				ID:             types.StringValue("garage"),
				AccessKey:      types.StringValue("access"),
				SecretKey:      types.StringValue("secret"),
				Endpoint:       types.StringValue("garage"),
				PathStyle:      types.BoolValue(false),
				ProviderQuirks: types.ListNull(types.StringType),
			}
			plan := tfsdk.Plan{Schema: schemaResponse.Schema}
			if diags := plan.Set(context.Background(), &base); diags.HasError() {
				t.Fatalf("setting test plan returned diagnostics: %#v", diags)
			}
			state := tfsdk.State{Schema: schemaResponse.Schema}
			stateData := base
			stateData.PathStyle = types.BoolValue(true)
			if diags := state.Set(context.Background(), &stateData); diags.HasError() {
				t.Fatalf("setting test state returned diagnostics: %#v", diags)
			}
			configPlan := tfsdk.Plan{Schema: schemaResponse.Schema}
			configData := base
			configData.PathStyle = test.configPathStyle
			if diags := configPlan.Set(context.Background(), &configData); diags.HasError() {
				t.Fatalf("setting test config returned diagnostics: %#v", diags)
			}

			response := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResponse.Schema}}
			resourceUnderTest := S3ConfigResource{client: client}
			resourceUnderTest.Update(context.Background(), resource.UpdateRequest{
				Config: tfsdk.Config{Raw: configPlan.Raw, Schema: schemaResponse.Schema},
				Plan:   plan,
				State:  state,
			}, &response)
			if response.Diagnostics.HasError() {
				t.Fatalf("unexpected update diagnostics: %#v", response.Diagnostics)
			}
		})
	}
}

func TestS3ConfigCreateSerializesBehindDigestMutation(t *testing.T) {
	ctx := context.Background()
	firstRead := make(chan struct{})
	createStarted := make(chan struct{})
	releaseFirstRead := make(chan struct{})
	interleaved := make(chan struct{})
	var interleavedOnce sync.Once
	var requestMu sync.Mutex
	existingGets := 0
	firstPutSeen := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/access/ticket":
			writeS3ConfigResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/config/s3/existing":
			requestMu.Lock()
			existingGets++
			getNumber := existingGets
			requestMu.Unlock()
			if getNumber == 1 {
				close(firstRead)
				<-createStarted
				<-releaseFirstRead
				writeS3ConfigResponseWithDigest(w, `{"id":"existing","access-key":"access","endpoint":"garage","region":"old"}`, "fresh-digest")
				return
			}
			writeS3ConfigResponse(w, `{"id":"existing","access-key":"access","endpoint":"garage","region":"new"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/config/s3/existing":
			if got, want := r.FormValue("digest"), "fresh-digest"; got != want {
				t.Errorf("unexpected update digest: got %q, want %q", got, want)
			}
			requestMu.Lock()
			firstPutSeen = true
			requestMu.Unlock()
			writeS3ConfigResponse(w, `null`)
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/config/s3":
			requestMu.Lock()
			mutationStarted := firstPutSeen
			requestMu.Unlock()
			if !mutationStarted {
				interleavedOnce.Do(func() { close(interleaved) })
			}
			writeS3ConfigResponse(w, `null`)
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/config/s3/new":
			writeS3ConfigResponse(w, `{"id":"new","access-key":"new-access","endpoint":"garage"}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	updatePlan := s3ConfigTestPlan(t, S3ConfigResourceModel{
		ID:             types.StringValue("existing"),
		AccessKey:      types.StringValue("access"),
		SecretKey:      types.StringValue("secret"),
		Endpoint:       types.StringValue("garage"),
		Region:         types.StringValue("new"),
		PathStyle:      types.BoolValue(false),
		ProviderQuirks: types.ListNull(types.StringType),
	})
	updateConfig := updatePlan
	updateState := s3ConfigTestState(t, S3ConfigResourceModel{
		ID:             types.StringValue("existing"),
		AccessKey:      types.StringValue("access"),
		SecretKey:      types.StringValue("secret"),
		Endpoint:       types.StringValue("garage"),
		Region:         types.StringValue("old"),
		PathStyle:      types.BoolValue(false),
		ProviderQuirks: types.ListNull(types.StringType),
	})
	createPlan := s3ConfigTestPlan(t, S3ConfigResourceModel{
		ID:             types.StringValue("new"),
		AccessKey:      types.StringValue("new-access"),
		SecretKey:      types.StringValue("new-secret"),
		Endpoint:       types.StringValue("garage"),
		ProviderQuirks: types.ListNull(types.StringType),
	})

	updateResponse := resource.UpdateResponse{State: updateState}
	updateResource := S3ConfigResource{client: client}
	var waitGroup sync.WaitGroup
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		updateResource.Update(ctx, resource.UpdateRequest{
			Config: tfsdk.Config(updateConfig),
			Plan:   updatePlan,
			State:  updateState,
		}, &updateResponse)
	}()

	select {
	case <-firstRead:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for update digest read")
	}
	createResponse := resource.CreateResponse{State: tfsdk.State(createPlan)}
	createResource := S3ConfigResource{client: client}
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		close(createStarted)
		createResource.Create(ctx, resource.CreateRequest{Plan: createPlan}, &createResponse)
	}()

	select {
	case <-interleaved:
		t.Error("S3 create interleaved before the digest mutation write")
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseFirstRead)
	waitGroup.Wait()
	if updateResponse.Diagnostics.HasError() {
		t.Fatalf("unexpected update diagnostics: %#v", updateResponse.Diagnostics)
	}
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("unexpected create diagnostics: %#v", createResponse.Diagnostics)
	}
}

func TestS3ConfigPayloadIncludesOptionalFields(t *testing.T) {
	data := S3ConfigResourceModel{
		ID:             types.StringValue("garage"),
		AccessKey:      types.StringValue("access"),
		SecretKey:      types.StringValue("secret"),
		Endpoint:       types.StringValue("garage"),
		Port:           types.Int64Value(3900),
		Region:         types.StringValue("garage"),
		Fingerprint:    types.StringValue("aa:bb"),
		PathStyle:      types.BoolValue(true),
		RateIn:         types.StringValue("1MiB"),
		BurstIn:        types.StringValue("2MiB"),
		RateOut:        types.StringValue("3MiB"),
		BurstOut:       types.StringValue("4MiB"),
		ProviderQuirks: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("skip-if-none-match-header")}),
		PutRateLimit:   types.Int64Value(100),
	}

	got, diags := s3ConfigPayload(context.Background(), data)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if got.Port == nil || *got.Port != 3900 {
		t.Fatalf("unexpected port: %#v", got.Port)
	}
	if got.Region == nil || *got.Region != "garage" {
		t.Fatalf("unexpected region: %#v", got.Region)
	}
	if got.PathStyle == nil || !*got.PathStyle {
		t.Fatalf("unexpected path-style: %#v", got.PathStyle)
	}
	if !reflect.DeepEqual(got.ProviderQuirks, []string{"skip-if-none-match-header"}) {
		t.Fatalf("unexpected provider quirks: %#v", got.ProviderQuirks)
	}
	if got.PutRateLimit == nil || *got.PutRateLimit != 100 {
		t.Fatalf("unexpected put rate limit: %#v", got.PutRateLimit)
	}
}

func TestReadS3ConfigPreservesEmptyProviderQuirksWhenAPIOmitsField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"ticket":"ticket-value","CSRFPreventionToken":"csrf-value"}}`))
		case "/api2/json/config/s3/garage":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"garage","access-key":"access","endpoint":"garage","port":3900}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	data := S3ConfigResourceModel{
		ID:             types.StringValue("garage"),
		ProviderQuirks: types.ListValueMust(types.StringType, []attr.Value{}),
	}
	resource := S3ConfigResource{client: client}

	if err := resource.readS3Config(context.Background(), &data); err != nil {
		t.Fatalf("readS3Config returned error: %s", err)
	}
	if data.ProviderQuirks.IsNull() {
		t.Fatal("provider_quirks became null")
	}
	if got := len(data.ProviderQuirks.Elements()); got != 0 {
		t.Fatalf("unexpected provider_quirks length: got %d, want 0", got)
	}
}

func TestReadS3ConfigPreservesConfiguredBandwidthWhenAPINormalizesSpacing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"ticket":"ticket-value","CSRFPreventionToken":"csrf-value"}}`))
		case "/api2/json/config/s3/garage":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"garage","access-key":"access","endpoint":"garage","rate-in":"1 MiB","burst-in":"2 MiB","rate-out":"3 MiB","burst-out":"4 MiB"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "secret", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}

	data := S3ConfigResourceModel{
		ID:       types.StringValue("garage"),
		RateIn:   types.StringValue("1MiB"),
		BurstIn:  types.StringValue("2MiB"),
		RateOut:  types.StringValue("3MiB"),
		BurstOut: types.StringValue("4MiB"),
	}
	resource := S3ConfigResource{client: client}

	if err := resource.readS3Config(context.Background(), &data); err != nil {
		t.Fatalf("readS3Config returned error: %s", err)
	}
	if got, want := data.RateIn.ValueString(), "1MiB"; got != want {
		t.Fatalf("unexpected rate_in: got %q, want %q", got, want)
	}
	if got, want := data.BurstIn.ValueString(), "2MiB"; got != want {
		t.Fatalf("unexpected burst_in: got %q, want %q", got, want)
	}
	if got, want := data.RateOut.ValueString(), "3MiB"; got != want {
		t.Fatalf("unexpected rate_out: got %q, want %q", got, want)
	}
	if got, want := data.BurstOut.ValueString(), "4MiB"; got != want {
		t.Fatalf("unexpected burst_out: got %q, want %q", got, want)
	}
}

func TestS3ConfigDeleteTransmitsFreshDigestAsForm(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeS3ConfigResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/s3/garage" {
				t.Fatalf("unexpected digest request: %s %s", r.Method, r.URL.Path)
			}
			writeS3ConfigResponseWithDigest(w, `{"id":"garage","access-key":"access","endpoint":"garage"}`, "fresh-digest")
		case 3:
			if r.Method != http.MethodDelete || r.URL.Path != "/api2/json/config/s3/garage" {
				t.Fatalf("unexpected delete request: %s %s", r.Method, r.URL.Path)
			}
			if got, want := r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"; got != want {
				t.Fatalf("unexpected delete content type: got %q, want %q", got, want)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read delete body: %s", err)
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("parse delete form: %s", err)
			}
			if got, want := form.Get("digest"), "fresh-digest"; got != want {
				t.Fatalf("unexpected delete digest: got %q, want %q", got, want)
			}
			writeS3ConfigResponse(w, `null`)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	state := s3ConfigTestState(t, S3ConfigResourceModel{
		ID:             types.StringValue("garage"),
		AccessKey:      types.StringValue("access"),
		SecretKey:      types.StringValue("secret"),
		Endpoint:       types.StringValue("garage"),
		ProviderQuirks: types.ListNull(types.StringType),
	})
	response := resource.DeleteResponse{}
	resourceUnderTest := S3ConfigResource{client: client}
	resourceUnderTest.Delete(context.Background(), resource.DeleteRequest{State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected delete diagnostics: %#v", response.Diagnostics)
	}
}

func TestS3ConfigCreatePersistsRecoveryStateAndRedactsCredentialsOnRefreshFailure(t *testing.T) {
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeS3ConfigResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/api2/json/config/s3" {
				t.Fatalf("unexpected create request: %s %s", r.Method, r.URL.Path)
			}
			writeS3ConfigResponse(w, `null`)
		case 3:
			if r.Method != http.MethodGet || r.URL.Path != "/api2/json/config/s3/garage" {
				t.Fatalf("unexpected refresh request: %s %s", r.Method, r.URL.Path)
			}
			http.Error(w, "access-key=access-value secret-key=secret-value", http.StatusInternalServerError)
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
	NewS3ConfigResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	planData := S3ConfigResourceModel{
		ID:             types.StringValue("garage"),
		AccessKey:      types.StringValue("access-value"),
		SecretKey:      types.StringValue("secret-value"),
		Endpoint:       types.StringValue("garage"),
		ProviderQuirks: types.ListNull(types.StringType),
	}
	if diags := plan.Set(context.Background(), &planData); diags.HasError() {
		t.Fatalf("setting test plan returned diagnostics: %#v", diags)
	}

	response := resource.CreateResponse{State: tfsdk.State(plan)}
	resourceUnderTest := S3ConfigResource{client: client}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected refresh error")
	}
	if got := response.Diagnostics[0].Detail(); strings.Contains(got, "access-value") || strings.Contains(got, "secret-value") {
		t.Fatalf("credentials were exposed in diagnostics: %q", got)
	}

	var recovered S3ConfigResourceModel
	if diags := response.State.Get(context.Background(), &recovered); diags.HasError() {
		t.Fatalf("reading recovery state returned diagnostics: %#v", diags)
	}
	if got, want := recovered.ID.ValueString(), "garage"; got != want {
		t.Fatalf("unexpected recovered ID: got %q, want %q", got, want)
	}
	if got, want := recovered.SecretKey.ValueString(), "secret-value"; got != want {
		t.Fatalf("unexpected recovered secret key: got %q, want %q", got, want)
	}
}

func TestS3ConfigCreateRedactsEncodedCredentialsFromDiagnostics(t *testing.T) {
	accessKey := `access/key+"&?`
	secretKey := `secret/key+"&?`
	credentialRepresentations := func(credential string) []string {
		jsonValue, err := json.Marshal(credential)
		if err != nil {
			t.Fatalf("marshal credential: %s", err)
		}
		quotedJSON := string(jsonValue)
		return []string{
			credential,
			url.QueryEscape(credential),
			url.PathEscape(credential),
			strconv.Quote(credential),
			quotedJSON,
			quotedJSON[1 : len(quotedJSON)-1],
		}
	}

	var errorBodyParts []string
	errorBodyParts = append(errorBodyParts, credentialRepresentations(accessKey)...)
	errorBodyParts = append(errorBodyParts, credentialRepresentations(secretKey)...)
	requestNumber := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			writeS3ConfigResponse(w, `{"ticket":"ticket","CSRFPreventionToken":"csrf"}`)
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/api2/json/config/s3" {
				t.Fatalf("unexpected create request: %s %s", r.Method, r.URL.Path)
			}
			http.Error(w, strings.Join(errorBodyParts, " "), http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request %d: %s %s", requestNumber, r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newProxmoxBackupServerClient(server.URL, "root@pam", "password", false)
	if err != nil {
		t.Fatalf("newProxmoxBackupServerClient returned error: %s", err)
	}
	plan := s3ConfigTestPlan(t, S3ConfigResourceModel{
		ID:             types.StringValue("garage"),
		AccessKey:      types.StringValue(accessKey),
		SecretKey:      types.StringValue(secretKey),
		Endpoint:       types.StringValue("garage"),
		ProviderQuirks: types.ListNull(types.StringType),
	})
	response := resource.CreateResponse{State: tfsdk.State(plan)}
	resourceUnderTest := S3ConfigResource{client: client}
	resourceUnderTest.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("expected create error")
	}
	diagnostic := response.Diagnostics[0].Detail()
	for _, representation := range errorBodyParts {
		if representation != "" && strings.Contains(diagnostic, representation) {
			t.Fatalf("credential representation was exposed in diagnostics: %q", representation)
		}
	}
}

func s3ConfigTestState(t *testing.T, data S3ConfigResourceModel) tfsdk.State {
	t.Helper()
	var schemaResponse resource.SchemaResponse
	NewS3ConfigResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	state := tfsdk.State{Schema: schemaResponse.Schema}
	if diags := state.Set(context.Background(), &data); diags.HasError() {
		t.Fatalf("setting test state returned diagnostics: %#v", diags)
	}
	return state
}

func s3ConfigTestPlan(t *testing.T, data S3ConfigResourceModel) tfsdk.Plan {
	t.Helper()
	var schemaResponse resource.SchemaResponse
	NewS3ConfigResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	if diags := plan.Set(context.Background(), &data); diags.HasError() {
		t.Fatalf("setting test plan returned diagnostics: %#v", diags)
	}
	return plan
}

func writeS3ConfigResponse(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"data":%s}`, data)
}

func writeS3ConfigResponseWithDigest(w http.ResponseWriter, data, digest string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"data":%s,"digest":%q}`, data, digest)
}
