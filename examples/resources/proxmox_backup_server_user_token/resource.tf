resource "proxmox_backup_server_user_token" "example" {
  user_id    = proxmox_backup_server_user.example.user_id
  token_name = "homepage"
  enable     = true
}
