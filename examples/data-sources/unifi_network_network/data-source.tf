# Look up a network by name (or by id) to reference its attributes elsewhere.
data "unifi_network_network" "lan" {
  name = "Default"
}

output "lan_subnet" {
  value = data.unifi_network_network.lan.subnet
}
