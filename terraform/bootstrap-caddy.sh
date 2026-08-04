#!/bin/sh
# Converts plaintext friend credentials into the bcrypt-hashed users.caddyfile
# Caddy actually reads (see the root Caddyfile's `import users.caddyfile`
# inside each basic_auth block) - see terraform/cloud-init.yaml.tftpl (which
# runs this after writing caddy/credentials.txt but before `docker compose
# up -d`) and the root CLAUDE.md's Caddy section for the full picture.
#
# credentials.txt (one "name:password" pair per line, written from
# CADDY_BASICAUTH_CREDENTIALS by cloud-init) never needs to persist across
# boots the way OpenBao's vault-init.env does - bootstrap-vault.sh keeps that
# file around because bao operator init is a one-time, irreversible action;
# hashing plaintext into bcrypt is trivially re-derivable from the GitHub
# secret on every recreate, so this script deletes it once it's done.
set -e
cd /opt/val-analyzer

CREDENTIALS_FILE=/opt/val-analyzer/caddy/credentials.txt
USERS_FILE=/opt/val-analyzer/caddy/users.caddyfile

: > "$USERS_FILE"

while IFS=: read -r name password; do
  [ -z "$name" ] && continue
  hash=$(docker run --rm caddy:2-alpine caddy hash-password --plaintext "$password")
  echo "$name $hash" >>"$USERS_FILE"
done <"$CREDENTIALS_FILE"

rm -f "$CREDENTIALS_FILE"
