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
	userid := "tfacc" + suffix + "@pbs"
	tokenName := "tfacc" + suffix
	tokenID := userid + "!" + tokenName

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAccessResourcesConfig(userid, tokenName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("proxmox_backup_server_user.test", "userid", userid),
					resource.TestCheckResourceAttr("proxmox_backup_server_user.test", "enable", "true"),
					resource.TestCheckResourceAttr("proxmox_backup_server_user.test", "comment", "Terraform acceptance test user"),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "id", tokenID),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "userid", userid),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "token_name", tokenName),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "enable", "true"),
					resource.TestCheckResourceAttr("proxmox_backup_server_user_token.test", "comment", "Terraform acceptance test token"),
					resource.TestCheckResourceAttrSet("proxmox_backup_server_user_token.test", "value"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "id", "/|"+userid+"|Audit"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "path", "/"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "auth_id", userid),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "role", "Audit"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.user", "propagate", "true"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "id", "/|"+tokenID+"|Audit"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "path", "/"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "auth_id", tokenID),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "role", "Audit"),
					resource.TestCheckResourceAttr("proxmox_backup_server_acl.token", "propagate", "true"),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_user.test", "userid", userid),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_user.test", "enable", "true"),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_user_token.test", "id", tokenID),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_user_token.test", "enable", "true"),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_acl.user", "auth_id", userid),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_acl.user", "role", "Audit"),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_acl.token", "auth_id", tokenID),
					resource.TestCheckResourceAttr("data.proxmox_backup_server_acl.token", "role", "Audit"),
				),
			},
			{
				ResourceName:                         "proxmox_backup_server_user.test",
				ImportState:                          true,
				ImportStateId:                        userid,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "userid",
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
				ImportStateId:     "/|" + userid + "|Audit",
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

func testAccAccessResourcesConfig(userid, tokenName string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "proxmox_backup_server_user" "test" {
  userid  = %[1]q
  enable  = true
  comment = "Terraform acceptance test user"
}

resource "proxmox_backup_server_user_token" "test" {
  userid     = proxmox_backup_server_user.test.userid
  token_name = %[2]q
  enable     = true
  comment    = "Terraform acceptance test token"
}

resource "proxmox_backup_server_acl" "user" {
  path    = "/"
  auth_id = proxmox_backup_server_user.test.userid
  role    = "Audit"
}

resource "proxmox_backup_server_acl" "token" {
  path    = "/"
  auth_id = proxmox_backup_server_user_token.test.id
  role    = "Audit"
}

data "proxmox_backup_server_user" "test" {
  userid = proxmox_backup_server_user.test.userid
}

data "proxmox_backup_server_user_token" "test" {
  userid     = proxmox_backup_server_user_token.test.userid
  token_name = proxmox_backup_server_user_token.test.token_name
}

data "proxmox_backup_server_acl" "user" {
  path    = proxmox_backup_server_acl.user.path
  auth_id = proxmox_backup_server_acl.user.auth_id
  role    = proxmox_backup_server_acl.user.role
}

data "proxmox_backup_server_acl" "token" {
  path    = proxmox_backup_server_acl.token.path
  auth_id = proxmox_backup_server_acl.token.auth_id
  role    = proxmox_backup_server_acl.token.role
}
`, userid, tokenName)
}
