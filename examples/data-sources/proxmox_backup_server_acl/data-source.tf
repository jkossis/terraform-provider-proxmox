data "proxmox_backup_server_acl" "example" {
  path    = "/"
  auth_id = "homepage@pbs"
  role    = "Audit"
}
