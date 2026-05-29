resource "proxmox_backup_server_datastore" "example" {
  name = "backup"
  path = "/mnt/datastore/backup"

  gc_schedule    = "daily"
  prune_schedule = "daily"
  keep_daily     = 7
  keep_weekly    = 4
  keep_monthly   = 6
}
