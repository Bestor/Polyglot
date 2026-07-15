output "droplet_ip" {
  description = "Public IPv4 address of the application droplet - consumed by the deploy job's SSH step."
  value       = digitalocean_droplet.app.ipv4_address
}
