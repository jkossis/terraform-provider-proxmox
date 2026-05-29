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

func TestAccS3ConfigResource(t *testing.T) {
	testAccPreCheck(t)

	resourceName := "proxmox_backup_server_s3_config.test"
	dataSourceName := "data.proxmox_backup_server_s3_config.test"
	s3ConfigID := "tfacc" + strconv.FormatInt(time.Now().UnixNano(), 36)
	s3Endpoint := os.Getenv("PROXMOX_BACKUP_SERVER_TEST_S3_ENDPOINT")
	s3AccessKey := os.Getenv("PROXMOX_BACKUP_SERVER_TEST_S3_ACCESS_KEY")
	s3SecretKey := os.Getenv("PROXMOX_BACKUP_SERVER_TEST_S3_SECRET_KEY")
	s3Port := os.Getenv("PROXMOX_BACKUP_SERVER_TEST_S3_PORT")
	s3Region := os.Getenv("PROXMOX_BACKUP_SERVER_TEST_S3_REGION")
	if s3Region == "" {
		s3Region = "us-east-1"
	}
	s3Fingerprint := os.Getenv("PROXMOX_BACKUP_SERVER_TEST_S3_FINGERPRINT")
	initialChecks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(resourceName, "id", s3ConfigID),
		resource.TestCheckResourceAttr(resourceName, "endpoint", s3Endpoint),
		resource.TestCheckResourceAttr(resourceName, "access_key", s3AccessKey),
		resource.TestCheckResourceAttr(resourceName, "region", s3Region),
		resource.TestCheckResourceAttr(resourceName, "path_style", "true"),
		resource.TestCheckResourceAttr(resourceName, "provider_quirks.0", "skip-if-none-match-header"),
		resource.TestCheckResourceAttr(resourceName, "put_rate_limit", "100"),
		resource.TestCheckResourceAttr(resourceName, "rate_in", "1MiB"),
		resource.TestCheckResourceAttr(resourceName, "burst_in", "2MiB"),
		resource.TestCheckResourceAttr(resourceName, "rate_out", "3MiB"),
		resource.TestCheckResourceAttr(resourceName, "burst_out", "4MiB"),
		resource.TestCheckResourceAttr(dataSourceName, "id", s3ConfigID),
		resource.TestCheckResourceAttr(dataSourceName, "endpoint", s3Endpoint),
		resource.TestCheckResourceAttr(dataSourceName, "access_key", s3AccessKey),
		resource.TestCheckResourceAttr(dataSourceName, "region", s3Region),
		resource.TestCheckResourceAttr(dataSourceName, "path_style", "true"),
		resource.TestCheckResourceAttr(dataSourceName, "provider_quirks.0", "skip-if-none-match-header"),
		resource.TestCheckResourceAttr(dataSourceName, "put_rate_limit", "100"),
		resource.TestCheckResourceAttr(dataSourceName, "rate_in", "1 MiB"),
		resource.TestCheckResourceAttr(dataSourceName, "burst_in", "2 MiB"),
		resource.TestCheckResourceAttr(dataSourceName, "rate_out", "3 MiB"),
		resource.TestCheckResourceAttr(dataSourceName, "burst_out", "4 MiB"),
	}
	if s3Port != "" {
		initialChecks = append(initialChecks,
			resource.TestCheckResourceAttr(resourceName, "port", s3Port),
			resource.TestCheckResourceAttr(dataSourceName, "port", s3Port),
		)
	}
	if s3Fingerprint != "" {
		initialChecks = append(initialChecks,
			resource.TestCheckResourceAttr(resourceName, "fingerprint", s3Fingerprint),
			resource.TestCheckResourceAttr(dataSourceName, "fingerprint", s3Fingerprint),
		)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccS3ConfigResourceConfig(s3ConfigID, s3Endpoint, s3AccessKey, s3SecretKey, s3Port, s3Region, s3Fingerprint, true) + testAccS3ConfigDataSourceConfig(),
				Check:  resource.ComposeAggregateTestCheckFunc(initialChecks...),
			},
			{
				Config: testAccS3ConfigResourceConfig(s3ConfigID, s3Endpoint, s3AccessKey+"-updated", s3SecretKey, "", "", "", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", s3ConfigID),
					resource.TestCheckResourceAttr(resourceName, "endpoint", s3Endpoint),
					resource.TestCheckResourceAttr(resourceName, "access_key", s3AccessKey+"-updated"),
					resource.TestCheckResourceAttr(resourceName, "path_style", "false"),
					resource.TestCheckNoResourceAttr(resourceName, "port"),
					resource.TestCheckNoResourceAttr(resourceName, "region"),
					resource.TestCheckNoResourceAttr(resourceName, "fingerprint"),
					resource.TestCheckNoResourceAttr(resourceName, "provider_quirks.#"),
					resource.TestCheckResourceAttr(resourceName, "put_rate_limit", "100"),
					resource.TestCheckNoResourceAttr(resourceName, "rate_in"),
					resource.TestCheckNoResourceAttr(resourceName, "burst_in"),
					resource.TestCheckNoResourceAttr(resourceName, "rate_out"),
					resource.TestCheckNoResourceAttr(resourceName, "burst_out"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secret_key"},
			},
		},
	})
}

func testAccProviderConfig() string {
	insecureTLS, _ := strconv.ParseBool(os.Getenv("PROXMOX_BACKUP_SERVER_INSECURE_TLS"))

	return fmt.Sprintf(`
provider "proxmox" {
	endpoint     = %[1]q
	username     = %[2]q
	password     = %[3]q
	insecure_tls = %[4]t
}
`, os.Getenv("PROXMOX_BACKUP_SERVER_ENDPOINT"), os.Getenv("PROXMOX_BACKUP_SERVER_USERNAME"), os.Getenv("PROXMOX_BACKUP_SERVER_PASSWORD"), insecureTLS)
}

func testAccS3ConfigResourceConfig(id, endpoint, accessKey, secretKey, port, region, fingerprint string, includeOptional bool) string {
	config := testAccProviderConfig() + fmt.Sprintf(`
resource "proxmox_backup_server_s3_config" "test" {
  id         = %[1]q
  endpoint   = %[2]q
  access_key = %[3]q
  secret_key = %[4]q
`, id, endpoint, accessKey, secretKey)

	if includeOptional {
		if port != "" {
			config += fmt.Sprintf("\n  port = %s\n", port)
		}
		if fingerprint != "" {
			config += fmt.Sprintf("\n  fingerprint = %q\n", fingerprint)
		}
		config += fmt.Sprintf(`
  region     = %[1]q
  path_style = true

  rate_in        = "1MiB"
  burst_in       = "2MiB"
  rate_out       = "3MiB"
  burst_out      = "4MiB"
  put_rate_limit = 100

  provider_quirks = [
    "skip-if-none-match-header",
  ]
`, region)
	}

	return config + "}\n"
}

func testAccS3ConfigDataSourceConfig() string {
	return `
data "proxmox_backup_server_s3_config" "test" {
  id = proxmox_backup_server_s3_config.test.id
}
`
}
