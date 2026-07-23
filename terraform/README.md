# One-time setup for the deploy pipeline

Everything under `terraform/` and `.github/workflows/deploy.yml` runs automatically on every push
to `main` (see the "Deployment (CI/CD)" section of the repo root `CLAUDE.md` for how it works).
None of that can bootstrap itself, though - Terraform can't create the backend it depends on, and
GitHub Actions needs secrets to exist before its first run. This is the checklist for everything
that has to happen once, by hand, before the pipeline can run at all.

Do these roughly in order - later steps need values produced by earlier ones.

## 1. DigitalOcean Space (Terraform state)

- [ ] In the DigitalOcean control panel: **Create -> Spaces Object Storage**.
  - Name: `polyglot-tfstate`
  - Region: **sfo3** - `terraform/main.tf`'s backend block is hardcoded to
    `https://sfo3.digitaloceanspaces.com`. DO Spaces regions each have their own separate bucket
    namespace (no cross-region lookup), so this must match exactly or `terraform init` fails with
    a `NoSuchBucket` 404 even though the bucket really exists, just in a different region. If you
    use a different region, update that endpoint in `terraform/main.tf` to match before the first
    `terraform init`.
  - **File Listing: Restrict File Listing** (i.e. private, not public) - this bucket will hold
    Terraform state, which contains every app secret in plaintext (that's inherent to how
    Terraform state works, not a mistake - see the plan's notes on this).
  - Enable versioning, so a corrupted/bad state write can be recovered.

## 2. DigitalOcean Spaces access keys (for Terraform's state backend)

- [ ] DigitalOcean control panel -> **API -> Spaces Keys -> Generate New Key**.
- [ ] Save both values immediately - the secret is only shown once.
  - Access key -> becomes the `SPACES_ACCESS_KEY_ID` GitHub secret
  - Secret key -> becomes the `SPACES_SECRET_ACCESS_KEY` GitHub secret

This is a **separate** credential from the general API token in the next step - Spaces uses
S3-style keys, not your DigitalOcean account token.

## 3. DigitalOcean API token (for Terraform's `digitalocean` provider)

- [ ] DigitalOcean control panel -> **API -> Tokens -> Generate New Token**.
  - Name: something identifiable, e.g. `val-analyzer-ci`
  - Scope: read + write (Terraform needs to create/update the droplet, volume, firewall, SSH key)
- [ ] Save the token -> becomes the `DIGITALOCEAN_TOKEN` GitHub secret.

## 4. Dedicated SSH keypair for CI deploys

Don't reuse your personal SSH key - generate one that only exists for this pipeline:

```sh
ssh-keygen -t ed25519 -C "val-analyzer-ci-deploy" -f ./val_analyzer_ci_deploy -N ""
```

This produces two files:
- [ ] `val_analyzer_ci_deploy.pub` -> becomes the `DEPLOY_SSH_PUBLIC_KEY` GitHub secret (Terraform
  registers this with DigitalOcean and installs it on the droplet - see
  `terraform/droplet.tf`'s `digitalocean_ssh_key.deploy`)
- [ ] `val_analyzer_ci_deploy` (the private half) -> becomes the `DEPLOY_SSH_PRIVATE_KEY` GitHub
  secret (used by the `deploy` job's SSH step)

Delete both local copies once they're saved as GitHub secrets - they don't need to live anywhere
else.

## 5. Make the repo public

`cloud-init.yaml.tftpl`'s `git clone`/`git pull` use a plain, unauthenticated HTTPS URL
(`https://github.com/Bestor/Polyglot.git`) - this only works if the repo is public. A
private repo fails here non-interactively (git tries to prompt for a username with no TTY to read
from), which silently prevents `/opt/val-analyzer` from ever being created and cascades into every
later `runcmd` step failing too.

- [ ] GitHub repo -> **Settings -> General -> Danger Zone -> Change visibility -> Make public**.

## 6. Confirm the droplet image slug is still current

`terraform/droplet.tf` pins `image = "docker-20-04"` (DigitalOcean's Docker-preinstalled Ubuntu
marketplace image). These slugs occasionally change - confirm it's still valid before the first
apply:

```sh
doctl compute image list-distribution --public | grep -i docker
```

If it's drifted, update the `image` value in `terraform/droplet.tf` to match before pushing.

## 7. Add every secret to GitHub

`.github/workflows/deploy.yml`'s `provision` and `deploy` jobs both declare `environment:
production`, so these secrets **must** go under a GitHub Environment named exactly `production`
(Settings -> Environments -> New environment -> name it `production` -> add secrets there), not
plain repository secrets - a job only sees Environment secrets if it explicitly declares that
environment, and GitHub silently resolves an unset/wrongly-scoped secret to an empty string rather
than failing the workflow, which is a confusing failure mode if you get this wrong (surfaces later,
as an opaque Terraform/AWS credential error). This also leaves room to add required-reviewer
protection on deploys later without touching the workflow.

- [ ] `DIGITALOCEAN_TOKEN` (step 3)
- [ ] `SPACES_ACCESS_KEY_ID` (step 2)
- [ ] `SPACES_SECRET_ACCESS_KEY` (step 2)
- [ ] `DEPLOY_SSH_PUBLIC_KEY` (step 4)
- [ ] `DEPLOY_SSH_PRIVATE_KEY` (step 4)
- [ ] `API_AUTH_TOKEN` - same value you'd use locally in `.env`; this becomes both `polyglot`'s and
  `mcpserver`'s shared bearer token in production
- [ ] `HENRIK_API_KEY` - cmd/valorantapi's own boot-time config since the two-binary split
- [ ] `DISCORD_BOT_TOKEN`
- [ ] `ANTHROPIC_API_KEY`
- [ ] `SUPERUSER_EMAIL` (optional - leave the GitHub secret unset/empty if you don't want an admin
  superuser auto-provisioned)
- [ ] `SUPERUSER_PASSWORD` (required if `SUPERUSER_EMAIL` is set, otherwise optional)

Notably absent: no `VAULT_TOKEN`/`VAULT_UNSEAL_KEY` secret to create here. OpenBao's one-time init/
policy setup is fully automated by `terraform/bootstrap-vault.sh` (see step 9) - those two values
are generated on the droplet itself and never leave it, so there's nothing to add to GitHub.

`GITHUB_TOKEN` (used to push to GHCR) is automatic - you don't create this one.

## 8. Push to `main` and handle the expected first-run hiccups

- [ ] Push to `main`. Watch the Actions tab - `build` should succeed and create the
  `ghcr.io/bestor/polyglot` package (as **private** by default).
- [ ] **The `deploy` job will likely fail on this very first run** - the droplet has no registry
  credentials (by design, see `CLAUDE.md`), so `docker compose pull` fails against a still-private
  image. This is expected, not a bug.
- [ ] Once `build` has run at least once: GitHub -> your profile/org -> **Packages ->
  polyglot -> Package settings -> Change visibility -> Public**. This can only be done
  after the package exists, which is why it can't happen earlier in this checklist.
- [ ] Re-run the failed workflow (or push an empty commit). `deploy` should now succeed, and
  `polyglot` should come up clean on the first try - see step 9 for how.

## 9. OpenBao setup and operations

OpenBao (the self-hosted Vault fork every onboarded datasource's secrets live in - see
`internal/vault` and the root `CLAUDE.md`) needs a few one-time steps before `polyglot` can boot -
initializing it (with a single key share/threshold, deliberately, so the one resulting key can
power fully automatic unsealing on every restart), enabling the KV v2 engine, and creating a
narrowly-scoped policy/token for `polyglot` itself (never the root token). **This is fully
automated** by `terraform/cloud-init.yaml.tftpl` + `terraform/bootstrap-vault.sh`, which run on
first boot: OpenBao comes up alone first, `bootstrap-vault.sh` runs the whole init/unseal/policy
sequence against it, writes the resulting scoped token + unseal key to
`/mnt/val-analyzer-data/vault-init.env` (a file on the same `prevent_destroy`-protected volume
OpenBao's own data lives on - not GitHub secrets, not Terraform state), appends them to `.env`, and
only then does the rest of the stack start. A droplet recreate reconnects to the same
already-initialized vault via that same persisted file instead of re-running `bao operator init`
(which errors if run against an already-initialized vault) or losing access to what's stored in it.
No by-hand SSH steps, nothing to add to GitHub secrets.

**If it ever doesn't come up clean** (e.g. `polyglot` crash-looping on `VAULT_TOKEN is required` -
check `docker logs val-analyzer-polyglot`): `bootstrap-vault.sh`'s 60-second wait loop for OpenBao's
listener may have timed out on an unusually slow boot. SSH in and re-run it by hand - it's
idempotent, safe to run again:
```sh
cd /opt/val-analyzer && ./terraform/bootstrap-vault.sh && docker compose up -d
```
If you ever need to run the underlying steps manually instead (e.g. debugging, or recovering a
vault whose `vault-init.env` was somehow lost while the underlying OpenBao data was not) the exact
commands are in `terraform/bootstrap-vault.sh` itself - it's a plain shell script, not a black box.

**Recurring operational note, not a one-time step**: OpenBao's file storage backend has no
persisted seal state, so it starts sealed again on every real restart (droplet reboot, or an
explicit `docker compose restart openbao`) - polyglot's own boot sequence unseals it automatically
using `VAULT_UNSEAL_KEY` (read from `.env`, populated by `bootstrap-vault.sh` at first boot) every
time it starts, so this is normally invisible. It only becomes visible (and needs a manual
`docker compose exec openbao bao operator unseal <key from vault-init.env>`) if OpenBao is
restarted *without* polyglot restarting alongside it.

## 10. Verify

- [ ] `terraform output droplet_ip` (from the `provision` job's logs, or run locally against the
  same backend) gives you the droplet's address.
- [ ] SSH in (`ssh -i val_analyzer_ci_deploy root@<droplet_ip>`) and confirm `docker compose ps`
  shows all six services up (`valorantapi`, `openbao`, `polyglot`, `mcpserver`, `cachewarmer`, and
  `discordbot` if the profile is active).
- [ ] `docker logs val-analyzer-mcpserver` shows tools being registered from `openapi/polyglot.yaml`.
- [ ] `GET /query?datasource=valorant&sql=SELECT COUNT(*) FROM matches` against `polyglot` (through
  its own bearer token) returns a real count, confirming the `polyglot` -> `openbao` +
  `polyglot` -> `valorantapi` wiring all actually works end to end.
- [ ] From your own machine, confirm nothing but SSH (22) is reachable on the droplet's public IP.

## Forcing a droplet rebuild

A plain push does **not** reliably rebuild the droplet just because `cloud-init.yaml.tftpl` changed
or an app secret was rotated - the DigitalOcean provider's `user_data` field suppresses that diff
in practice despite being schema-`ForceNew` (see `CLAUDE.md`'s Deployment section). If you need the
droplet actually rebuilt (e.g. right after fixing something in `cloud-init.yaml.tftpl`, or after
rotating a secret): GitHub repo -> **Actions -> Deploy -> Run workflow**, toggle
`recreate_droplet` to true, run. The existing `val-analyzer-data` volume (`pb_data`/
`polyglot_metadata`/`openbao_file` subdirectories) reattaches automatically
(`prevent_destroy`-protected, per `terraform/volume.tf`), so this never loses cached data - though
OpenBao does come back up sealed and needs polyglot's own restart (which happens automatically as
part of the same `docker compose up`) to auto-unseal it again.
