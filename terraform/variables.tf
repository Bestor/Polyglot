# Non-secret infrastructure knobs, defaulted for a small personal deployment.

variable "droplet_region" {
  description = "DigitalOcean region slug for the droplet and volume. Colocated with sfo3, where the polyglot-tfstate Space (Terraform state, terraform/main.tf) lives - not a hard requirement, just avoids unnecessary cross-region latency/complexity."
  type        = string
  default     = "sfo3"
}

variable "droplet_size" {
  description = "DigitalOcean droplet size slug. s-1vcpu-1gb is enough for 4 lightweight Go binaries + SQLite at personal-project scale."
  type        = string
  default     = "s-1vcpu-1gb"
}

variable "volume_size_gb" {
  description = "Size of the persistent data volume backing PocketBase's SQLite data. Grow-only if resized later."
  type        = number
  default     = 5
}

variable "anthropic_model" {
  description = "Model cmd/discordbot's tool-use loop calls (ANTHROPIC_MODEL). cmd/discordbot itself defaults to claude-opus-4-8 when this is unset - the default here is a deliberate, explicit production choice, not the binary silently picking a cheaper model on its own."
  type        = string
  default     = "claude-haiku-4-5"
}

# --- SSH ---

variable "do_ssh_public_key" {
  description = "Public half of the dedicated CI deploy keypair, registered with DigitalOcean and installed on the droplet."
  type        = string
}

# --- App secrets - flow into cloud-init.yaml.tftpl and become the droplet's
# .env. Every one of these forces a droplet recreate if its value changes,
# since it's baked into user_data (immutable post-boot) - see main plan doc
# for why that's the intended behavior, not a bug. ---

variable "api_auth_token" {
  description = "polyglot/mcpserver shared bearer token (API_AUTH_TOKEN)."
  type        = string
  sensitive   = true
}

variable "henrik_api_key" {
  description = "HenrikDev API key for the valorant DataProvider."
  type        = string
  sensitive   = true
}

variable "discord_bot_token" {
  description = "Discord bot token (cmd/discordbot)."
  type        = string
  sensitive   = true
}

variable "anthropic_api_key" {
  description = "Anthropic API key for the Discord bot's tool-use loop."
  type        = string
  sensitive   = true
}

variable "superuser_email" {
  description = "Optional PocketBase admin superuser email. Leave blank to skip auto-provisioning."
  type        = string
  sensitive   = true
  default     = ""
}

variable "superuser_password" {
  description = "Optional PocketBase admin superuser password. Required if superuser_email is set."
  type        = string
  sensitive   = true
  default     = ""
}
