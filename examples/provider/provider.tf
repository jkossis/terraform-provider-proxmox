terraform {
  required_providers {
    proxmox = {
      source = "jkossis/proxmox"
    }
  }
}

provider "proxmox" {
  endpoint = "https://backup.example.com:8007"
  username = "root@pam"
  password = var.proxmox_backup_server_password

  # Only use this for lab/self-signed installations.
  # insecure_tls = true
}
