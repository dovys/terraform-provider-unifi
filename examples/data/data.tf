# Data sources: look up existing objects and controller facts to reference
# elsewhere in your configuration.

# Look up a single network by name (or by id).
data "unifi_network_network" "lan" {
  name = "Default"
}

output "lan_subnet" {
  value = data.unifi_network_network.lan.subnet
}

# Enumerate every network on the configured site.
data "unifi_network_networks" "all" {}

output "network_names" {
  value = [for n in data.unifi_network_networks.all.networks : n.name]
}

# Network name => controller id. These ids are what go in an `import {}` block's
# `id` for unifi_network_network and unifi_network_wan (WANs are networkconf
# documents too, so they appear here). Run `terraform apply` and read this
# output, then copy the id you need.
output "network_ids" {
  value = { for n in data.unifi_network_networks.all.networks : n.name => n.id }
}

# Resolve the configured site (or pass `name` for another slug) to its id.
data "unifi_network_site" "current" {}

output "site_id" {
  value = data.unifi_network_site.current.id
}

# Read the detected controller version (from the Configure-time probe) — useful
# for gating configuration on firmware support.
data "unifi_network_application_info" "this" {}

output "controller_version" {
  value = data.unifi_network_application_info.this.version
}

# Look up a device by MAC (or by id) to reference its telemetry without managing
# it — e.g. to read an access point's current IP or firmware version.
data "unifi_network_device" "ap" {
  mac = "00:11:22:33:44:66"
}

output "ap_ip" {
  value = data.unifi_network_device.ap.ip
}

output "ap_version" {
  value = data.unifi_network_device.ap.version
}
