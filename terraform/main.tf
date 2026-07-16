# Backend + provider configuration for val-analyzer's droplet/volume/firewall.
#
# State lives in a DigitalOcean Space (S3-compatible object storage), not
# Terraform Cloud - one provider to manage instead of two. The Space itself
# ("polyglot-tfstate") is created once, by hand, before this backend can
# be used at all (see terraform/README.md) - Terraform can't bootstrap the
# backend it depends on.
#
# use_lockfile gives real state locking via conditional writes, with no
# DynamoDB-equivalent needed - this is what actually prevents two overlapping
# `terraform apply` runs from corrupting state; the GitHub Actions workflow's
# `concurrency` gate is a second, free layer on top of this, not a substitute
# for it.
terraform {
  required_version = ">= 1.10"

  required_providers {
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.0"
    }
  }

  backend "s3" {
    bucket = "polyglot-tfstate"
    key    = "val-analyzer/terraform.tfstate"
    region = "us-east-1" # required by the backend's schema; DO ignores it, paired with skip_region_validation below

    endpoints = {
      s3 = "https://sfo3.digitaloceanspaces.com" # must match whichever region the Space was created in - polyglot-tfstate lives in sfo3
    }

    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    skip_requesting_account_id  = true # DO Spaces has no STS/IAM API - without this, init tries (and fails) an AWS account-ID lookup via GetCallerIdentity/ListRoles
    use_lockfile                = true
  }
}

# Token read automatically from the DIGITALOCEAN_TOKEN environment variable -
# no explicit token argument needed (verified against the provider's docs).
provider "digitalocean" {}
