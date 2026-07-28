# Legacy path|user_id|role_id ACL ID:
terraform import 'proxmox_backup_server_acl.example' '/|homepage@pbs|Audit'

# Typed path|ugid_type|user_id|role_id ACL ID:
terraform import 'proxmox_backup_server_acl.group' '/|group|backup-admins|Audit'
