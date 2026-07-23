# The persistent home for valorantapi's cached PocketBase data, polyglot's
# own (smaller) onboarding/catalog bookkeeping, and OpenBao's file storage
# backend - three subdirectories on one disk (pb_data, polyglot_metadata,
# openbao_file; see cloud-init.yaml.tftpl's mkdir/mount steps and
# docker-compose.yml's *_HOST_PATH overrides). Kept as its own resource,
# decoupled from the droplet, specifically so the droplet can be treated as
# disposable/reprovisionable without losing the cache that the whole
# project's design exists to build up - see CLAUDE.md's "core design
# constraint" framing.
resource "digitalocean_volume" "data" {
  region                  = var.droplet_region
  name                    = "val-analyzer-data"
  size                    = var.volume_size_gb
  initial_filesystem_type = "ext4"

  lifecycle {
    prevent_destroy = true
  }
}
