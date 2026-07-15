# The persistent home for PocketBase's SQLite data (see docker-compose.yml's
# PB_DATA_HOST_PATH override and cloud-init.yaml.tftpl's mount step). Kept as
# its own resource, decoupled from the droplet, specifically so the droplet
# can be treated as disposable/reprovisionable without losing the cache that
# the whole project's design exists to build up - see CLAUDE.md's "core
# design constraint" framing.
resource "digitalocean_volume" "data" {
  region                  = var.droplet_region
  name                    = "val-analyzer-data"
  size                    = var.volume_size_gb
  initial_filesystem_type = "ext4"

  lifecycle {
    prevent_destroy = true
  }
}
