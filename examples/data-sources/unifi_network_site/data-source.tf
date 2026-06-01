# Resolve the configured site (or pass `name` for another slug) to its UUID.
data "unifi_network_site" "current" {}

output "site_uuid" {
  value = data.unifi_network_site.current.id
}
