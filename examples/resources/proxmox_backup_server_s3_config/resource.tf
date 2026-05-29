resource "proxmox_backup_server_s3_config" "example" {
  id         = "backup-s3"
  endpoint   = "s3.example.com"
  access_key = "example-access-key"
  secret_key = var.s3_secret_key

  region     = "us-east-1"
  path_style = true

  provider_quirks = [
    "skip-if-none-match-header",
  ]
}
