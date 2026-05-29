resource "proxmox_backup_server_user_token" "example" {
  userid     = proxmox_backup_server_user.example.userid
  token_name = "homepage"
  enable     = true
}
