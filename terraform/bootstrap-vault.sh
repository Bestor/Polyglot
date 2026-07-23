#!/bin/sh
# Bootstraps OpenBao on first boot (or after a droplet recreate) so
# polyglot never needs VAULT_TOKEN/VAULT_UNSEAL_KEY supplied by hand -
# see terraform/cloud-init.yaml.tftpl (which runs this after `docker
# compose up -d openbao` but before the rest of the stack starts) and the
# root CLAUDE.md's OpenBao section for the full picture.
#
# Persists the resulting scoped token + single unseal key to a file on
# the same prevent_destroy-protected DigitalOcean volume OpenBao's own
# data lives on (terraform/volume.tf) - never in GitHub secrets or
# Terraform state/variables - so a droplet recreate reconnects to the
# already-initialized vault using that same file instead of re-running
# `bao operator init` (which errors: a vault can only ever be initialized
# once) or permanently losing access to whatever was already stored in
# it. The root token generated below is used only transiently, within
# this script, to create a narrowly-scoped policy/token - it is never
# written anywhere.
set -e
cd /opt/val-analyzer

# Wait for OpenBao's own listener to come up - it may still be sealed or
# entirely uninitialized at this point, that's fine, this just waits for
# the process itself to be reachable via `docker compose exec`. "bao
# status" prints the literal word "Initialized" (as a status field,
# regardless of its true/false value) as soon as the server responds at
# all, so grepping for it is a reliable "is it up yet" signal distinct
# from "is it initialized yet".
for i in $(seq 1 30); do
  if docker compose exec -T openbao bao status 2>/dev/null | grep -q "Initialized"; then
    break
  fi
  sleep 2
done

INIT_FILE=/mnt/val-analyzer-data/vault-init.env

if [ ! -f "$INIT_FILE" ]; then
  # True first-ever init: this volume has never held OpenBao state
  # before. Single key share/threshold deliberately, so the one
  # resulting key can auto-unseal on every future restart with no human
  # involved (internal/vault does this on polyglot's own boot, every
  # time it starts).
  INIT_OUT=$(docker compose exec -T openbao bao operator init -key-shares=1 -key-threshold=1)
  UNSEAL_KEY=$(echo "$INIT_OUT" | grep "Unseal Key 1:" | awk '{print $NF}')
  ROOT_TOKEN=$(echo "$INIT_OUT" | grep "Initial Root Token:" | awk '{print $NF}')

  docker compose exec -T openbao bao operator unseal "$UNSEAL_KEY" >/dev/null
  docker compose exec -T -e BAO_TOKEN="$ROOT_TOKEN" openbao bao secrets enable -path=secret kv-v2 >/dev/null

  docker compose exec -T -e BAO_TOKEN="$ROOT_TOKEN" openbao sh -c 'cat <<POLICY | bao policy write polyglot -
path "secret/data/datasources/*" { capabilities = ["create", "read", "update", "delete"] }
path "secret/metadata/datasources/*" { capabilities = ["read", "delete", "list"] }
POLICY' >/dev/null

  SCOPED_TOKEN=$(docker compose exec -T -e BAO_TOKEN="$ROOT_TOKEN" openbao bao token create -policy=polyglot -field=token)

  cat > "$INIT_FILE" <<EOF
VAULT_TOKEN=$SCOPED_TOKEN
VAULT_UNSEAL_KEY=$UNSEAL_KEY
EOF
  chmod 600 "$INIT_FILE"
fi

# Idempotent even if this script is ever re-run by hand (e.g. manual
# recovery after the wait loop above timed out on a slow boot) - never
# appends a second copy.
grep -q '^VAULT_TOKEN=' /opt/val-analyzer/.env || cat "$INIT_FILE" >> /opt/val-analyzer/.env
