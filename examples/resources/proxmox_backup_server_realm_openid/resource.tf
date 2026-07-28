resource "proxmox_backup_server_realm_openid" "example" {
  realm      = "openid"
  issuer_url = "https://identity.example.com"
  client_id  = "proxmox-backup-server"
  client_key = var.openid_client_key

  scopes         = "email profile"
  prompt         = "login"
  comment        = "Managed by Terraform"
  autocreate     = true
  username_claim = "preferred_username"
}
