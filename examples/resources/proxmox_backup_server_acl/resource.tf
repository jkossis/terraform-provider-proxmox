resource "proxmox_backup_server_acl" "example" {
  path      = "/"
  user_id   = proxmox_backup_server_user.example.user_id
  role_id   = "Audit"
  propagate = true
}
