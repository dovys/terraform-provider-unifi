# UniFi OS console certificate — installs a TLS cert on the console itself (the
# unifi-core management UI, e.g. https://192.0.2.1), over the one console session
# (no api_key). Requires Terraform >= 1.11 / OpenTofu >= 1.9 (write-only args).
#
# For source = letsEncrypt the console owns renewal: Terraform issues the cert
# once and hands the DNS credentials to the device, which renews on its own. A
# plain `terraform apply` never re-issues — the DNS credentials are write-only
# (never stored in state, never diffed). To rotate a token or switch provider,
# recreate the resource: `terraform apply -replace=unifi_console_certificate.console`.

# Let's Encrypt via Cloudflare DNS-01 (the most common setup). The console places
# the _acme-challenge TXT in the zone itself using this token — so only the public
# TXT is needed, not a public A record for the console.
resource "unifi_console_certificate" "console" {
  name   = "console"
  source = "letsEncrypt"
  domain = "console.example.com"

  dns_provider = {
    name      = "cloudflare"
    api_token = var.cloudflare_api_token # write-only: never stored in state
  }
}

variable "cloudflare_api_token" {
  type        = string
  sensitive   = true
  description = "Cloudflare API token with Zone:DNS:Edit on the domain's zone."
}

# Resolve the certificate's domain to the console's LAN IP, so the name (and its
# now-valid certificate) is reachable from inside the network without a public A
# record. Pairs with the certificate above — same domain, the console's address.
resource "unifi_network_dns_record" "console" {
  name  = unifi_console_certificate.console.domain
  type  = "A"
  value = "192.0.2.1" # the console's LAN IP (your gateway's address)
}

# Same, but with AWS Route 53 — the credential set differs per provider
# (cloudflare/digitalocean: api_token; route53: access_key_id/secret_access_key/region;
#  godaddy: api_key/api_secret; porkbun: api_key/secret_key; ovh: application_key/
#  application_secret/consumer_key/endpoint; duckdns: token; dynu: api_key).
#
# resource "unifi_console_certificate" "console_r53" {
#   name   = "console"
#   domain = "console.example.com"
#   dns_provider = {
#     name              = "route53"
#     access_key_id     = var.aws_access_key_id
#     secret_access_key = var.aws_secret_access_key
#     region            = "us-east-1"
#   }
# }

# Manual DNS-01: the device cannot place the TXT for you, so you must publish the
# _acme-challenge record yourself (e.g. via your DNS provider's own Terraform
# provider). Use an automated provider above for hands-off operation.
#
# resource "unifi_console_certificate" "console_manual" {
#   name         = "console"
#   domain       = "console.example.com"
#   dns_provider = { name = "manual" }
# }

# Bring-your-own certificate (upload). The PEM chains with an upstream cert
# resource: when `certificate` changes the cert is re-uploaded. `private_key` is
# write-only (never stored in state).
#
# resource "unifi_console_certificate" "console_byo" {
#   name        = "console"
#   source      = "upload"
#   certificate = file("${path.module}/console.fullchain.pem")
#   private_key = file("${path.module}/console.key.pem")
# }
