# Enumerate every network on the configured site.
data "unifi_network_networks" "all" {}

output "network_names" {
  value = [for n in data.unifi_network_networks.all.networks : n.name]
}
