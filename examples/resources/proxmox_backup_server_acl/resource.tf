resource "proxmox_backup_server_acl" "example" {
  path      = "/"
  auth_id   = proxmox_backup_server_user.example.userid
  role      = "Audit"
  propagate = true
}
