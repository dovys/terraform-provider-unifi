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

# Resolve the configured site (or pass `name` for another slug) to its UUID.
data "unifi_network_site" "current" {}

output "site_uuid" {
  value = data.unifi_network_site.current.id
}

# Read the detected controller version and capabilities (from the
# Configure-time probe) — useful for gating configuration on firmware support.
data "unifi_network_application_info" "this" {}

output "controller_version" {
  value = data.unifi_network_application_info.this.version
}

output "integration_writes_supported" {
  value = data.unifi_network_application_info.this.integration_writes
}
