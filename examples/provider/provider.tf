terraform {
  required_providers {
    unifi = {
      source = "dovys/unifi"
    }
  }
}

# A local UniFi OS admin account is effectively required (cloud/SSO accounts
# that force 2FA are rejected). An Integration API key is optional and unlocks
# the official API where firmware supports it. All fields also read from
# environment variables (UNIFI_HOST, UNIFI_USERNAME, UNIFI_PASSWORD, …).
provider "unifi" {
  host     = "https://192.0.2.1"
  username = var.unifi_username
  password = var.unifi_password
  api_key  = var.unifi_api_key

  # UniFi consoles ship self-signed certs on local IPs.
  allow_insecure = true
}
