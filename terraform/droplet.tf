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
  name   = "val-analyzer"
  image  = "docker-20-04" # DigitalOcean's Docker-preinstalled Ubuntu marketplace slug - confirm the current exact slug (`doctl compute image list-distribution`) before first apply, these drift
  region = var.droplet_region
  size   = var.droplet_size

  ssh_keys   = [digitalocean_ssh_key.deploy.fingerprint]
  volume_ids = [digitalocean_volume.data.id]

  # Runs on first boot only (and on the rare real recreate) - mounts the
  # attached volume, writes .env from the variables above, clones the repo,
  # and brings the stack up. See cloud-init.yaml.tftpl.
  user_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    api_auth_token     = var.api_auth_token
    henrik_api_key     = var.henrik_api_key
    discord_bot_token  = var.discord_bot_token
    anthropic_api_key  = var.anthropic_api_key
    anthropic_model    = var.anthropic_model
    superuser_email    = var.superuser_email
    superuser_password = var.superuser_password
  })
}

# Nothing in this stack needs to be internet-facing: discordbot only makes
# outbound connections (Discord Gateway, Anthropic API), and
# mcpserver/polyglot are only reached over the internal Compose network. SSH
# is the only inbound rule.
resource "digitalocean_firewall" "app" {
  name        = "val-analyzer-ssh-only"
  droplet_ids = [digitalocean_droplet.app.id]

  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
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
