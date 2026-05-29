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

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](https://go.dev/doc/install) installed on your machine.

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```
