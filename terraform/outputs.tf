output "droplet_ip" {
  description = "Stable public IPv4 address of the application droplet (the reserved IP, not the droplet's own transient one - see digitalocean_reserved_ip.app in droplet.tf) - consumed by the deploy job's SSH step and by terraform/dns.tf's records, so both keep working across a droplet recreate."
  value       = digitalocean_reserved_ip.app.ip_address
}
