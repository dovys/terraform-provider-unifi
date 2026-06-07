# Let's Encrypt cert on the console UI itself (https://192.168.30.1) via native
# ACME DNS-01. The console owns renewal; rotate the token with -replace.
resource "unifi_console_certificate" "console" {
  name   = "console"
  source = "letsEncrypt"
  domain = "console.home.example"

  dns_provider = {
    name      = "cloudflare"
    api_token = var.dns_api_token # write-only: never stored in state
  }
}
