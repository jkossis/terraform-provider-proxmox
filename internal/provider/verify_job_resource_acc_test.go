// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccVerifyJobResource(t *testing.T) {
	testAccPreCheck(t)
	store := os.Getenv("PROXMOX_BACKUP_SERVER_TEST_VERIFY_DATASTORE")
	if store == "" {
		t.Skip("PROXMOX_BACKUP_SERVER_TEST_VERIFY_DATASTORE must name an existing datastore.")
	}
	id := "tfacc" + strconv.FormatInt(time.Now().UnixNano(), 36)
	config := testAccProviderConfig() + fmt.Sprintf(`
resource "proxmox_backup_server_verify_job" "test" {
  id    = %q
  store = %q
  %%s
}
`, id, store)
	name := "proxmox_backup_server_verify_job.test"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// A distant schedule exercises configuration without launching verification.
				Config: fmt.Sprintf(config, `schedule = "2099-01-01 09:00"
  comment = "Verification acceptance test"
  ignore_verified = false
  outdated_after = 30
  max_depth = 0
  read_threads = 1
  verify_threads = 2`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", id),
					resource.TestCheckResourceAttr(name, "store", store),
					resource.TestCheckResourceAttr(name, "schedule", "2099-01-01 09:00"),
					resource.TestCheckResourceAttr(name, "ignore_verified", "false"),
					resource.TestCheckResourceAttr(name, "outdated_after", "30"),
				),
			},
			{ResourceName: name, ImportState: true, ImportStateId: id, ImportStateVerify: true},
			{
				Config: fmt.Sprintf(config, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(name, "schedule"),
					resource.TestCheckNoResourceAttr(name, "comment"),
					resource.TestCheckNoResourceAttr(name, "ignore_verified"),
					resource.TestCheckNoResourceAttr(name, "outdated_after"),
					resource.TestCheckNoResourceAttr(name, "max_depth"),
					resource.TestCheckNoResourceAttr(name, "read_threads"),
					resource.TestCheckNoResourceAttr(name, "verify_threads"),
				),
			},
		},
	})
}
