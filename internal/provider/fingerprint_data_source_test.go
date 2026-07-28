// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestFingerprintDataSourceSchemaDocumentsRawTLSLeafBehavior(t *testing.T) {
	var resp datasource.SchemaResponse
	NewFingerprintDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	fingerprint, ok := resp.Schema.Attributes["fingerprint"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("fingerprint has type %T, want schema.StringAttribute", resp.Schema.Attributes["fingerprint"])
	}

	for name, description := range map[string]string{
		"data source": resp.Schema.MarkdownDescription,
		"fingerprint": fingerprint.MarkdownDescription,
	} {
		for _, phrase := range []string{
			"first (leaf) TLS certificate",
			"configured HTTPS endpoint",
			"bypasses certificate verification",
			"trust-on-first-use (TOFU)",
			"reverse-proxy certificate rather than a PBS node certificate",
		} {
			if !strings.Contains(description, phrase) {
				t.Errorf("%s description does not document %q: %q", name, phrase, description)
			}
		}
	}

	if !fingerprint.Computed {
		t.Fatal("fingerprint is not computed")
	}
	if !strings.Contains(fingerprint.MarkdownDescription, "uppercase colon-separated hex") {
		t.Fatalf("fingerprint description does not document uppercase colon-separated output: %q", fingerprint.MarkdownDescription)
	}
}

func TestSHA256FingerprintUsesUppercaseColonSeparatedHex(t *testing.T) {
	if got, want := sha256Fingerprint([]byte("fingerprint-test")), "7F:F8:B8:26:2F:3A:02:B5:A2:1E:95:FF:DC:48:6D:41:AD:98:32:46:73:CA:93:F5:B0:E4:20:33:44:09:CB:8D"; got != want {
		t.Fatalf("unexpected fingerprint: got %q, want %q", got, want)
	}
}

func TestFingerprintDataSource(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(server.Close)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testFingerprintDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.proxmox_backup_server_fingerprint.test", "fingerprint", sha256Fingerprint(server.Certificate().Raw)),
				),
			},
		},
	})
}

func testFingerprintDataSourceConfig(endpoint string) string {
	return fmt.Sprintf(`
provider "proxmox" {
  endpoint     = %[1]q
  username     = "root@pam"
  password     = "secret"
  insecure_tls = true
}

data "proxmox_backup_server_fingerprint" "test" {}
`, endpoint)
}
