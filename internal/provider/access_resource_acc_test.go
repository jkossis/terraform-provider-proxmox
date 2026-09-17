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

func TestAccAccessResources(t *testing.T) {
	testAccPreCheck(t)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	userID := "tfacc" + suffix + "@pbs"
	tokenName := "tfacc" + suffix
	tokenID := userID + "!" + tokenName

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAccessResourcesConfig(userID, tokenName, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("proxmox_backup_server_user.test", "user_id", userID),
					resource.TestCheckResourceAttr("proxmox_backup_server_user.test", "enable", "true"),
					resource.TestCheckResourceAttr("proxmox_backup_server_user.test", "comment", "Terraform acceptance test user"),
					resource.TestCheckResourceAttr("proxmox_backup_server_user.test", "password", "tfacc-password-"+suffix),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "id", tokenID),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "user_id", userID),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "token_name", tokenName),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "enable", "true"),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "comment", "Terraform acceptance test token"),
					resource.TestCheckResourceAttrSet("proxmox_backup_server_user_token.test", "value"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "id", "/|"+userID+"|Audit"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "path", "/"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "user_id", userID),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "role_id", "Audit"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "propagate", "true"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "id", "/|"+tokenID+"|Audit"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "path", "/"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "user_id", tokenID),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "role_id", "Audit"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "propagate", "true"),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_user.test", "user_id", userID),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_user.test", "enable", "true"),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_user_token.test", "id", tokenID),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_user_token.test", "enable", "true"),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_acl.user", "user_id", userID),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_acl.user", "role_id", "Audit"),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_acl.token", "user_id", tokenID),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_acl.token", "role_id", "Audit"),
				),
			},
			{
				ResourceName:                         "proxmox_backup_server_user.test",
				ImportState:                          true,
				ImportStateId:                        userID,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "user_id",
				// The API never returns the password.
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				ResourceName:            "proxmox_backup_server_user_token.test",
				ImportState:             true,
				ImportStateId:           tokenID,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value"},
			},
			{
				ResourceName:      "proxmox_backup_server_acl.user",
				ImportState:       true,
				ImportStateId:     "/|" + userID + "|Audit",
				ImportStateVerify: true,
			},
			{
				ResourceName:      "proxmox_backup_server_acl.token",
				ImportState:       true,
				ImportStateId:     "/|" + tokenID + "|Audit",
				ImportStateVerify: true,
			},
		},
	})
}

func testAccAccessResourcesConfig(userID, tokenName, suffix string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "proxmox_backup_server_user" "test" {
  user_id  = %[1]q
  enable   = true
  comment  = "Terraform acceptance test user"
  password = "tfacc-password-%[3]s"
}

resource "proxmox_backup_server_user_token" "test" {
  user_id    = proxmox_backup_server_user.test.user_id
  token_name = %[2]q
  enable     = true
  comment    = "Terraform acceptance test token"
}

resource "proxmox_backup_server_acl" "user" {
  path    = "/"
  user_id = proxmox_backup_server_user.test.user_id
  role_id = "Audit"
}

resource "proxmox_backup_server_acl" "token" {
  path    = "/"
  user_id = proxmox_backup_server_user_token.test.id
  role_id = "Audit"
}

data "proxmox_backup_server_user" "test" {
  user_id = proxmox_backup_server_user.test.user_id
}

data "proxmox_backup_server_user_token" "test" {
  user_id    = proxmox_backup_server_user_token.test.user_id
  token_name = proxmox_backup_server_user_token.test.token_name
}

data "proxmox_backup_server_acl" "user" {
  path    = proxmox_backup_server_acl.user.path
  user_id = proxmox_backup_server_acl.user.user_id
  role_id = proxmox_backup_server_acl.user.role_id
}

data "proxmox_backup_server_acl" "token" {
  path    = proxmox_backup_server_acl.token.path
  user_id = proxmox_backup_server_acl.token.user_id
  role_id = proxmox_backup_server_acl.token.role_id
}
`, userID, tokenName, suffix)
}
