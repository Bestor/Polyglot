# DNS for the Caddy public entrypoint (see docker-compose.yml's caddy
# service and the root Caddyfile). All three records point at the droplet's
# stable digitalocean_reserved_ip (droplet.tf), not its own ipv4_address, so
# they keep resolving correctly across a droplet recreate. proxied = false
# throughout - DNS-only ("grey cloud"), a deliberate choice so Caddy owns
# its own Let's Encrypt certificate directly (TLS-ALPN-01, port 443 only)
# rather than trusting Cloudflare's proxy as a second TLS-terminating hop.

data "cloudflare_zone" "app" {
  filter = {
    name = var.domain_name
  }
}

resource "cloudflare_dns_record" "root" {
  zone_id = data.cloudflare_zone.app.id
  name    = "@"
  type    = "A"
  content = digitalocean_reserved_ip.app.ip_address
  proxied = false
  ttl     = 300
}

resource "cloudflare_dns_record" "mcp" {
  zone_id = data.cloudflare_zone.app.id
  name    = "mcp"
  type    = "A"
  content = digitalocean_reserved_ip.app.ip_address
  proxied = false
  ttl     = 300
}

resource "cloudflare_dns_record" "traces" {
  zone_id = data.cloudflare_zone.app.id
  name    = "traces"
  type    = "A"
  content = digitalocean_reserved_ip.app.ip_address
  proxied = false
  ttl     = 300
}
