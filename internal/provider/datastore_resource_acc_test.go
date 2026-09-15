// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestAccDatastoreResource(t *testing.T) {
	testAccPreCheck(t)

	pathPrefix := os.Getenv("PROXMOX_BACKUP_SERVER_TEST_DATASTORE_PATH_PREFIX")
	if pathPrefix == "" {
		t.Skip("PROXMOX_BACKUP_SERVER_TEST_DATASTORE_PATH_PREFIX must be set to run datastore acceptance tests")
	}

	resourceName := "proxmox_backup_server_datastore.test"
	dataSourceName := "data.proxmox_backup_server_datastore.test"
	datastoreName := "tfacc" + strconv.FormatInt(time.Now().UnixNano(), 36)
	datastorePath := path.Join(pathPrefix, datastoreName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDatastoreResourceConfig(datastoreName, datastorePath, true) + testAccDatastoreDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", datastoreName),
					resource.TestCheckResourceAttr(resourceName, "path", datastorePath),
					resource.TestCheckResourceAttr(resourceName, "comment", "Terraform acceptance test"),
					resource.TestCheckResourceAttr(resourceName, "gc_schedule", "daily"),
					resource.TestCheckResourceAttr(resourceName, "verify_new", "true"),
					resource.TestCheckResourceAttr(dataSourceName, "name", datastoreName),
					resource.TestCheckResourceAttr(dataSourceName, "path", datastorePath),
					resource.TestCheckResourceAttr(dataSourceName, "comment", "Terraform acceptance test"),
					resource.TestCheckResourceAttr(dataSourceName, "gc_schedule", "daily"),
					resource.TestCheckResourceAttr(dataSourceName, "verify_new", "true"),
				),
			},
			{
				Config: testAccDatastoreResourceConfig(datastoreName, datastorePath, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", datastoreName),
					resource.TestCheckResourceAttr(resourceName, "path", datastorePath),
					resource.TestCheckResourceAttr(resourceName, "comment", "Terraform acceptance test updated"),
					resource.TestCheckNoResourceAttr(resourceName, "gc_schedule"),
					resource.TestCheckNoResourceAttr(resourceName, "verify_new"),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportState:                          true,
				ImportStateId:                        datastoreName,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
		},
	})
}

func testAccDatastoreResourceConfig(name, datastorePath string, includeOptional bool) string {
	config := testAccProviderConfig() + fmt.Sprintf(`
resource "proxmox_backup_server_datastore" "test" {
  name = %[1]q
  path = %[2]q
`, name, datastorePath)

	if includeOptional {
		config += `
  comment     = "Terraform acceptance test"
  gc_schedule = "daily"
  verify_new  = true
`
	} else {
		config += `
  comment = "Terraform acceptance test updated"
`
	}

	return config + "}\n"
}

func testAccDatastoreDataSourceConfig() string {
	return `
data "proxmox_backup_server_datastore" "test" {
  name = proxmox_backup_server_datastore.test.name
}
`
}
