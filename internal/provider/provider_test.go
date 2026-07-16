// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories is used to instantiate a provider during acceptance testing.
// The factory function is called for each Terraform CLI command to create a provider
// server that the CLI can connect to and interact with.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	providerTypeName: providerserver.NewProtocol6WithError(New("test")()),
}

var requiredAcceptanceEnvVars = []string{
	proxmoxEndpointEnv,
	proxmoxUsernameEnv,
	proxmoxPasswordEnv,
}

func testAccPreCheck(t *testing.T) {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set to run acceptance tests")
	}

	missing := missingAcceptanceEnvVars(os.Getenv)
	if len(missing) > 0 {
		t.Fatalf("acceptance tests require environment variables: %s", strings.Join(missing, ", "))
	}
}

func testAccProviderConfig() string {
	return `
provider "proxmox" {}
`
}

func testAccS3PreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)

	required := []string{
		"PROXMOX_BACKUP_SERVER_TEST_S3_ENDPOINT",
		"PROXMOX_BACKUP_SERVER_TEST_S3_ACCESS_KEY",
		"PROXMOX_BACKUP_SERVER_TEST_S3_SECRET_KEY",
	}
	missing := make([]string, 0, len(required))
	for _, envVar := range required {
		if os.Getenv(envVar) == "" {
			missing = append(missing, envVar)
		}
	}
	if len(missing) > 0 {
		t.Skipf("S3 acceptance tests require environment variables: %s", strings.Join(missing, ", "))
	}
}

func missingAcceptanceEnvVars(lookupEnv func(string) string) []string {
	missing := make([]string, 0, len(requiredAcceptanceEnvVars))
	for _, envVar := range requiredAcceptanceEnvVars {
		if lookupEnv(envVar) == "" {
			missing = append(missing, envVar)
		}
	}
	return missing
}
