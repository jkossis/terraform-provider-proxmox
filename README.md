# Terraform Provider for Proxmox

Terraform provider for managing Proxmox Backup Server resources.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.25

## Building the Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Using the Provider

```terraform
terraform {
  required_providers {
    proxmox = {
      source = "jkossis/proxmox"
    }
  }
}

provider "proxmox" {
  endpoint = "https://backup.example.com:8007"
  username = "root@pam"
  password = var.proxmox_backup_server_password
}
```

Provider configuration can also be supplied with environment variables:

| Attribute | Environment variable | Required |
| --- | --- | --- |
| `endpoint` | `PROXMOX_ENDPOINT` | Yes |
| `username` | `PROXMOX_USERNAME` | Yes |
| `password` | `PROXMOX_PASSWORD` | Yes |
| `insecure_tls` | `PROXMOX_INSECURE_TLS` | No |

Explicit provider configuration takes precedence over environment variables. `PROXMOX_INSECURE_TLS` accepts standard Terraform/Go boolean strings such as `true` or `false`.

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](https://go.dev/doc/install) installed on your machine.

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

In order to run the full suite of acceptance tests, set `PROXMOX_ENDPOINT`, `PROXMOX_USERNAME`, and `PROXMOX_PASSWORD`, then run `mise run testacc`. The task sets `TF_ACC=1`. `PROXMOX_INSECURE_TLS` is optional and should only be used for lab or self-signed Proxmox Backup Server installations.

Datastore acceptance tests also require `PROXMOX_BACKUP_SERVER_TEST_DATASTORE_PATH_PREFIX`. Import datastores by name, for example: `terraform import proxmox_backup_server_datastore.example backup`.

Verification-job acceptance tests require `PROXMOX_BACKUP_SERVER_TEST_VERIFY_DATASTORE`
to name an existing datastore. They create a uniquely named job with a schedule
in 2099, test import and removal of optional settings, then delete the job.
They do not start verification or alter backup data. Credentials may be supplied
through the shell or ignored `mise.local.toml`.

S3 configuration acceptance tests run only when `PROXMOX_BACKUP_SERVER_TEST_S3_ENDPOINT`, `PROXMOX_BACKUP_SERVER_TEST_S3_ACCESS_KEY`, and `PROXMOX_BACKUP_SERVER_TEST_S3_SECRET_KEY` are set. Optional S3 variables are `PROXMOX_BACKUP_SERVER_TEST_S3_PORT`, `PROXMOX_BACKUP_SERVER_TEST_S3_REGION`, and `PROXMOX_BACKUP_SERVER_TEST_S3_FINGERPRINT`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
mise run testacc
```
