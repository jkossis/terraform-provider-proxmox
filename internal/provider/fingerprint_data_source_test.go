// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

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
