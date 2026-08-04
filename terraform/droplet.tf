# The application droplet, its SSH access, and its firewall. Deliberately
# idempotent: `terraform apply` on every push is a no-op here unless this
# resource's own definition changed (size/region/image, or user_data content
# via a rotated secret - see variables.tf) - see the CI/CD plan doc for why
# that's the intended "infra as code without needless churn" behavior.

resource "digitalocean_ssh_key" "deploy" {
  name       = "val-analyzer-ci-deploy"
  public_key = var.do_ssh_public_key
}

resource "digitalocean_droplet" "app" {
  name   = "polyglot"
  image  = "docker-20-04" # DigitalOcean's Docker-preinstalled Ubuntu marketplace slug - confirm the current exact slug (`doctl compute image list-distribution`) before first apply, these drift
  region = var.droplet_region
  size   = var.droplet_size

  ssh_keys   = [digitalocean_ssh_key.deploy.fingerprint]
  volume_ids = [digitalocean_volume.data.id]

  # Runs on first boot only (and on the rare real recreate) - mounts the
  # attached volume, writes .env from the variables above, clones the repo,
  # and brings the stack up. See cloud-init.yaml.tftpl.
  user_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    api_auth_token              = var.api_auth_token
    henrik_api_key              = var.henrik_api_key
    discord_bot_token           = var.discord_bot_token
    anthropic_api_key           = var.anthropic_api_key
    anthropic_model             = var.anthropic_model
    superuser_email             = var.superuser_email
    superuser_password          = var.superuser_password
    domain_name                 = var.domain_name
    acme_email                  = var.acme_email
    caddy_basicauth_credentials = var.caddy_basicauth_credentials
  })
}

# A stable public IP that survives a droplet recreate (e.g. via
# recreate_droplet: true / -replace). Without this, terraform/dns.tf's
# Cloudflare records would silently point at a dead IP after every recreate
# until someone noticed and manually repointed them. droplet_id attaches it
# directly to the droplet - the alternative digitalocean_reserved_ip_assignment
# resource is only needed if you ever want to detach/reattach independently
# of this resource's own lifecycle, which nothing here does.
resource "digitalocean_reserved_ip" "app" {
  region     = var.droplet_region
  droplet_id = digitalocean_droplet.app.id
}

# Public entrypoint is now Caddy on 443 (see docker-compose.yml's caddy
# service and the root Caddyfile) - everything else stays reachable only
# over the internal Compose network by another service. discordbot only
# makes outbound connections.
resource "digitalocean_firewall" "app" {
  name        = "polyglot-ssh-and-https"
  droplet_ids = [digitalocean_droplet.app.id]

  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
    source_addresses = ["0.0.0.0/0", "::/0"]
  }

  inbound_rule {
    protocol         = "tcp"
    port_range       = "443"
    source_addresses = ["0.0.0.0/0", "::/0"]
  }

  outbound_rule {
    protocol              = "tcp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }

  outbound_rule {
    protocol              = "udp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }
}
