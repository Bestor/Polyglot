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
  - Region: **nyc3** - `terraform/main.tf`'s backend block is hardcoded to
    `https://nyc3.digitaloceanspaces.com`. If you use a different region, update that endpoint to
    match before the first `terraform init`.
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

## 5. Confirm the droplet image slug is still current

`terraform/droplet.tf` pins `image = "docker-20-04"` (DigitalOcean's Docker-preinstalled Ubuntu
marketplace image). These slugs occasionally change - confirm it's still valid before the first
apply:

```sh
doctl compute image list-distribution --public | grep -i docker
```

If it's drifted, update the `image` value in `terraform/droplet.tf` to match before pushing.

## 6. Add every secret to GitHub

Repo -> **Settings -> Secrets and variables -> Actions**. I'd recommend creating a `production`
Environment first (Settings -> Environments -> New environment) and adding these there rather than
as plain repo secrets - costs nothing extra and leaves room to add required-reviewer protection on
deploys later without changing the workflow.

- [ ] `DIGITALOCEAN_TOKEN` (step 3)
- [ ] `SPACES_ACCESS_KEY_ID` (step 2)
- [ ] `SPACES_SECRET_ACCESS_KEY` (step 2)
- [ ] `DEPLOY_SSH_PUBLIC_KEY` (step 4)
- [ ] `DEPLOY_SSH_PRIVATE_KEY` (step 4)
- [ ] `API_AUTH_TOKEN` - same value you'd use locally in `.env`; this becomes both `polyglot`'s and
  `mcpserver`'s shared bearer token in production
- [ ] `HENRIK_API_KEY`
- [ ] `DISCORD_BOT_TOKEN`
- [ ] `ANTHROPIC_API_KEY`
- [ ] `SUPERUSER_EMAIL` (optional - leave the GitHub secret unset/empty if you don't want an admin
  superuser auto-provisioned)
- [ ] `SUPERUSER_PASSWORD` (required if `SUPERUSER_EMAIL` is set, otherwise optional)

`GITHUB_TOKEN` (used to push to GHCR) is automatic - you don't create this one.

## 7. Push to `main` and handle the expected first-run hiccup

- [ ] Push to `main`. Watch the Actions tab - `build` should succeed and create the
  `ghcr.io/bestor/valorantanalyzer` package (as **private** by default).
- [ ] **The `deploy` job will likely fail on this very first run** - the droplet has no registry
  credentials (by design, see `CLAUDE.md`), so `docker compose pull` fails against a still-private
  image. This is expected, not a bug.
- [ ] Once `build` has run at least once: GitHub -> your profile/org -> **Packages ->
  valorantanalyzer -> Package settings -> Change visibility -> Public**. This can only be done
  after the package exists, which is why it can't happen earlier in this checklist.
- [ ] Re-run the failed workflow (or push an empty commit). `deploy` should now succeed.

## 8. Verify

- [ ] `terraform output droplet_ip` (from the `provision` job's logs, or run locally against the
  same backend) gives you the droplet's address.
- [ ] SSH in (`ssh -i val_analyzer_ci_deploy root@<droplet_ip>`) and confirm `docker compose ps`
  shows all four services up.
- [ ] `docker logs val-analyzer-mcpserver` shows `tools=6`.
- [ ] From your own machine, confirm nothing but SSH (22) is reachable on the droplet's public IP.
