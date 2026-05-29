// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
