data "proxmox_backup_server_acl" "example" {
  path    = "/"
  user_id = "homepage@pbs"
  role_id = "Audit"
}
