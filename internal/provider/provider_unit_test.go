// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	tfprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProviderMetadata_preservesProviderTypeName(t *testing.T) {
	provider := &ProxmoxBackupServerProvider{version: "test-version"}
	var resp tfprovider.MetadataResponse

	provider.Metadata(context.Background(), tfprovider.MetadataRequest{}, &resp)

	if got, want := resp.TypeName, "proxmox"; got != want {
		t.Fatalf("unexpected provider type name: got %q, want %q", got, want)
	}
	if got, want := resp.Version, "test-version"; got != want {
		t.Fatalf("unexpected provider version: got %q, want %q", got, want)
	}
}

func TestProviderSchema_usesOptionalEnvironmentBackedAttributes(t *testing.T) {
	provider := &ProxmoxBackupServerProvider{}
	var resp tfprovider.SchemaResponse

	provider.Schema(context.Background(), tfprovider.SchemaRequest{}, &resp)

	assertOptionalStringAttribute(t, stringAttributeExpectation{attributes: resp.Schema.Attributes, name: "endpoint", envVar: proxmoxEndpointEnv})
	assertOptionalStringAttribute(t, stringAttributeExpectation{attributes: resp.Schema.Attributes, name: "username", envVar: proxmoxUsernameEnv})
	assertOptionalStringAttribute(t, stringAttributeExpectation{attributes: resp.Schema.Attributes, name: "password", envVar: proxmoxPasswordEnv, sensitive: true})
	attribute, ok := resp.Schema.Attributes["insecure_tls"].(providerschema.BoolAttribute)
	if !ok {
		t.Fatalf("insecure_tls attribute has type %T, want schema.BoolAttribute", resp.Schema.Attributes["insecure_tls"])
	}
	if !attribute.Optional || attribute.Required {
		t.Fatalf("insecure_tls optional/required mismatch: optional=%t required=%t", attribute.Optional, attribute.Required)
	}
	if !strings.Contains(attribute.MarkdownDescription, proxmoxInsecureTLSEnv) {
		t.Fatalf("insecure_tls description does not mention %s: %q", proxmoxInsecureTLSEnv, attribute.MarkdownDescription)
	}
}

func TestProviderConfig_usesExplicitConfigBeforeEnvironmentFallback(t *testing.T) {
	data := ProxmoxBackupServerProviderModel{
		Endpoint:    types.StringValue("https://configured.example.com:8007"),
		Username:    types.StringValue("configured@pam"),
		Password:    types.StringValue("configured-password"),
		InsecureTLS: types.BoolValue(false),
	}

	config, diags := providerConfigFrom(data, func(name string) string {
		return map[string]string{
			proxmoxEndpointEnv:    "https://env.example.com:8007",
			proxmoxUsernameEnv:    "env@pam",
			proxmoxPasswordEnv:    "env-password",
			proxmoxInsecureTLSEnv: "true",
		}[name]
	})

	assertNoDiagnostics(t, diags)
	if got, want := config.endpoint, "https://configured.example.com:8007"; got != want {
		t.Fatalf("unexpected endpoint: got %q, want %q", got, want)
	}
	if got, want := config.username, "configured@pam"; got != want {
		t.Fatalf("unexpected username: got %q, want %q", got, want)
	}
	if got, want := config.password, "configured-password"; got != want {
		t.Fatalf("unexpected password: got %q, want %q", got, want)
	}
	if config.insecureTLS {
		t.Fatalf("expected explicit insecure_tls=false to override %s=true", proxmoxInsecureTLSEnv)
	}
}

func TestProviderConfig_usesEnvironmentFallbackWhenConfigIsUnset(t *testing.T) {
	data := ProxmoxBackupServerProviderModel{
		Endpoint:    types.StringNull(),
		Username:    types.StringNull(),
		Password:    types.StringNull(),
		InsecureTLS: types.BoolNull(),
	}

	config, diags := providerConfigFrom(data, func(name string) string {
		return map[string]string{
			proxmoxEndpointEnv:    "https://env.example.com:8007",
			proxmoxUsernameEnv:    "env@pam",
			proxmoxPasswordEnv:    "env-password",
			proxmoxInsecureTLSEnv: "true",
		}[name]
	})

	assertNoDiagnostics(t, diags)
	if got, want := config.endpoint, "https://env.example.com:8007"; got != want {
		t.Fatalf("unexpected endpoint: got %q, want %q", got, want)
	}
	if got, want := config.username, "env@pam"; got != want {
		t.Fatalf("unexpected username: got %q, want %q", got, want)
	}
	if got, want := config.password, "env-password"; got != want {
		t.Fatalf("unexpected password: got %q, want %q", got, want)
	}
	if !config.insecureTLS {
		t.Fatalf("expected %s=true to set insecureTLS", proxmoxInsecureTLSEnv)
	}
}

func TestProviderConfig_returnsAttributeDiagnosticsWhenRequiredValuesAreMissing(t *testing.T) {
	data := ProxmoxBackupServerProviderModel{
		Endpoint:    types.StringNull(),
		Username:    types.StringNull(),
		Password:    types.StringNull(),
		InsecureTLS: types.BoolNull(),
	}

	_, diags := providerConfigFrom(data, func(string) string { return "" })

	assertDiagnosticPaths(t, diags, path.Root("endpoint"), path.Root("username"), path.Root("password"))
}

func TestProviderConfig_doesNotUseEnvironmentFallbackForUnknownValues(t *testing.T) {
	data := ProxmoxBackupServerProviderModel{
		Endpoint:    types.StringUnknown(),
		Username:    types.StringUnknown(),
		Password:    types.StringUnknown(),
		InsecureTLS: types.BoolUnknown(),
	}

	config, diags := providerConfigFrom(data, func(string) string { return "true" })

	assertDiagnosticPaths(t, diags, path.Root("endpoint"), path.Root("username"), path.Root("password"), path.Root("insecure_tls"))
	if config.endpoint != "" || config.username != "" || config.password != "" || config.insecureTLS {
		t.Fatalf("unknown configuration must not use environment values: %#v", config)
	}
}

func TestProviderConfig_rejectsEmptyExplicitStringValues(t *testing.T) {
	data := ProxmoxBackupServerProviderModel{
		Endpoint:    types.StringValue(""),
		Username:    types.StringValue(""),
		Password:    types.StringValue(""),
		InsecureTLS: types.BoolNull(),
	}

	config, diags := providerConfigFrom(data, func(name string) string {
		if name == proxmoxInsecureTLSEnv {
			return "true"
		}
		return "from-environment"
	})

	assertDiagnosticPaths(t, diags, path.Root("endpoint"), path.Root("username"), path.Root("password"))
	if config.endpoint != "" || config.username != "" || config.password != "" || !config.insecureTLS {
		t.Fatalf("empty explicit values must not use string environment values: %#v", config)
	}
}

func TestProviderConfig_returnsAttributeDiagnosticForInvalidInsecureTLSEnvironmentValue(t *testing.T) {
	data := ProxmoxBackupServerProviderModel{
		Endpoint:    types.StringValue("https://configured.example.com:8007"),
		Username:    types.StringValue("configured@pam"),
		Password:    types.StringValue("configured-password"),
		InsecureTLS: types.BoolNull(),
	}

	_, diags := providerConfigFrom(data, func(name string) string {
		if name == proxmoxInsecureTLSEnv {
			return "not-a-bool"
		}
		return ""
	})

	assertDiagnosticPaths(t, diags, path.Root("insecure_tls"))
}

func TestProviderResources_preservePublicTypeNames(t *testing.T) {
	provider := &ProxmoxBackupServerProvider{}

	got := resourceTypeNames(t, provider.Resources(context.Background()))
	want := []string{
		"proxmox_backup_server_acl",
		"proxmox_backup_server_datastore",
		"proxmox_backup_server_realm_openid",
		"proxmox_backup_server_s3_config",
		"proxmox_backup_server_user",
		"proxmox_backup_server_user_token",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected resource type names: got %#v, want %#v", got, want)
	}
}

func TestProviderDataSources_preservePublicTypeNames(t *testing.T) {
	provider := &ProxmoxBackupServerProvider{}

	got := dataSourceTypeNames(t, provider.DataSources(context.Background()))
	want := []string{
		"proxmox_backup_server_acl",
		"proxmox_backup_server_datastore",
		"proxmox_backup_server_fingerprint",
		"proxmox_backup_server_s3_config",
		"proxmox_backup_server_user",
		"proxmox_backup_server_user_token",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected data source type names: got %#v, want %#v", got, want)
	}
}

func TestAcceptancePreCheck_requiresOnlyCoreProviderEnvironmentVariables(t *testing.T) {
	got := missingAcceptanceEnvVars(func(name string) string {
		return map[string]string{
			proxmoxEndpointEnv:    "https://backup.example.com:8007",
			proxmoxUsernameEnv:    "root@pam",
			proxmoxPasswordEnv:    "secret",
			proxmoxInsecureTLSEnv: "not-required",
		}[name]
	})

	if len(got) != 0 {
		t.Fatalf("unexpected missing env vars: %#v", got)
	}

	got = missingAcceptanceEnvVars(func(string) string { return "" })
	want := []string{proxmoxEndpointEnv, proxmoxUsernameEnv, proxmoxPasswordEnv}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected required env vars: got %#v, want %#v", got, want)
	}
}

type stringAttributeExpectation struct {
	attributes map[string]providerschema.Attribute
	name       string
	envVar     string
	sensitive  bool
}

func assertOptionalStringAttribute(t *testing.T, expectation stringAttributeExpectation) {
	t.Helper()
	attribute, ok := expectation.attributes[expectation.name].(providerschema.StringAttribute)
	if !ok {
		t.Fatalf("%s attribute has type %T, want schema.StringAttribute", expectation.name, expectation.attributes[expectation.name])
	}
	if !attribute.Optional || attribute.Required {
		t.Fatalf("%s optional/required mismatch: optional=%t required=%t", expectation.name, attribute.Optional, attribute.Required)
	}
	if attribute.Sensitive != expectation.sensitive {
		t.Fatalf("%s sensitive mismatch: got %t, want %t", expectation.name, attribute.Sensitive, expectation.sensitive)
	}
	if !strings.Contains(attribute.MarkdownDescription, expectation.envVar) {
		t.Fatalf("%s description does not mention %s: %q", expectation.name, expectation.envVar, attribute.MarkdownDescription)
	}
}

func assertNoDiagnostics(t *testing.T, diags diag.Diagnostics) {
	t.Helper()
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
}

func assertDiagnosticPaths(t *testing.T, diags diag.Diagnostics, wantPaths ...path.Path) {
	t.Helper()
	if got, want := diags.ErrorsCount(), len(wantPaths); got != want {
		t.Fatalf("unexpected diagnostic count: got %d, want %d: %#v", got, want, diags)
	}
	for _, wantPath := range wantPaths {
		found := false
		for _, diagnostic := range diags {
			pathDiagnostic, ok := diagnostic.(diag.DiagnosticWithPath)
			if ok && pathDiagnostic.Path().Equal(wantPath) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing diagnostic path %s in %#v", wantPath.String(), diags)
		}
	}
}

func resourceTypeNames(t *testing.T, constructors []func() resource.Resource) []string {
	t.Helper()
	typeNames := make([]string, 0, len(constructors))
	for _, constructor := range constructors {
		var resp resource.MetadataResponse
		constructor().Metadata(context.Background(), resource.MetadataRequest{}, &resp)
		typeNames = append(typeNames, resp.TypeName)
	}
	return typeNames
}

func dataSourceTypeNames(t *testing.T, constructors []func() datasource.DataSource) []string {
	t.Helper()
	typeNames := make([]string, 0, len(constructors))
	for _, constructor := range constructors {
		var resp datasource.MetadataResponse
		constructor().Metadata(context.Background(), datasource.MetadataRequest{}, &resp)
		typeNames = append(typeNames, resp.TypeName)
	}
	return typeNames
}
