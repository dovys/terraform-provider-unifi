# host/site are non-secret. Username/password come from UNIFI_USERNAME /
# UNIFI_PASSWORD (a local UniFi OS admin; cookie session, no API key).
provider "unifi" {
  host           = var.unifi_host
  site           = var.unifi_site
  allow_insecure = true # self-signed cert on the LAN IP
}
