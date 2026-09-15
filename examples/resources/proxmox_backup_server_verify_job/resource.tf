resource "proxmox_backup_server_verify_job" "daily" {
  id              = "daily"
  store           = proxmox_backup_server_datastore.example.name
  schedule        = "09:00"
  ignore_verified = true
  outdated_after  = 30
}
