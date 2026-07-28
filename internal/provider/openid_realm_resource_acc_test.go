// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccOpenIDRealmResource(t *testing.T) {
	testAccPreCheck(t)

	resourceName := "proxmox_backup_server_realm_openid.test"
	realm := "tfacc" + strconv.FormatInt(time.Now().UnixNano(), 36)
	initialIssuerURL := "https://issuer.example.com"
	updatedIssuerURL := "https://issuer-updated.example.com"
	initialClientID := "tfacc-client-" + realm
	updatedClientID := initialClientID + "-updated"
	initialAudiences := "proxmox-backup"
	updatedAudiences := "proxmox-backup,terraform"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccOpenIDRealmResourceConfig(realm, initialIssuerURL, initialClientID, initialAudiences, "Terraform acceptance test", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "realm", realm),
					resource.TestCheckResourceAttr(resourceName, "id", realm),
					resource.TestCheckResourceAttr(resourceName, "issuer_url", initialIssuerURL),
					resource.TestCheckResourceAttr(resourceName, "client_id", initialClientID),
					resource.TestCheckResourceAttr(resourceName, "audiences", initialAudiences),
					resource.TestCheckResourceAttr(resourceName, "scopes", "email profile"),
					resource.TestCheckResourceAttr(resourceName, "autocreate", "false"),
					resource.TestCheckResourceAttr(resourceName, "default", "false"),
					resource.TestCheckResourceAttr(resourceName, "comment", "Terraform acceptance test"),
					resource.TestCheckResourceAttr(resourceName, "username_claim", "preferred_username"),
					resource.TestCheckResourceAttrSet(resourceName, "digest"),
				),
			},
			{
				Config: testAccOpenIDRealmResourceConfig(realm, updatedIssuerURL, updatedClientID, updatedAudiences, "Terraform acceptance test updated", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "realm", realm),
					resource.TestCheckResourceAttr(resourceName, "id", realm),
					resource.TestCheckResourceAttr(resourceName, "issuer_url", updatedIssuerURL),
					resource.TestCheckResourceAttr(resourceName, "client_id", updatedClientID),
					resource.TestCheckResourceAttr(resourceName, "audiences", updatedAudiences),
					resource.TestCheckResourceAttr(resourceName, "scopes", "email profile"),
					resource.TestCheckResourceAttr(resourceName, "autocreate", "true"),
					resource.TestCheckResourceAttr(resourceName, "default", "false"),
					resource.TestCheckResourceAttr(resourceName, "comment", "Terraform acceptance test updated"),
					resource.TestCheckResourceAttr(resourceName, "username_claim", "preferred_username"),
					resource.TestCheckResourceAttrSet(resourceName, "digest"),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportState:                          true,
				ImportStateId:                        realm,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "realm",
				ImportStateVerifyIgnore:              []string{"digest"},
			},
		},
	})
}

func testAccOpenIDRealmResourceConfig(realm, issuerURL, clientID, audiences, comment string, autocreate bool) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "proxmox_backup_server_realm_openid" "test" {
  realm         = %[1]q
  issuer_url    = %[2]q
  client_id     = %[3]q
  audiences     = %[4]q
  scopes        = "email profile"
  comment       = %[5]q
  autocreate    = %[6]t
  default       = false
  username_claim = "preferred_username"
}
`, realm, issuerURL, clientID, audiences, comment, autocreate)
}
