# Firewall use case: stop the IoT VLAN from initiating connections to the
# trusted main LAN, while still allowing the main LAN to reach IoT devices.
#
# Classic firewall rules act on a ruleset (chain). LAN_IN evaluates traffic as it
# enters the router from a local network, so a drop rule there blocks IoT→Main
# new connections; established/related return traffic for Main→IoT still flows.

# A reusable group of the trusted LAN subnets the IoT network must not reach.
# Reference it from rules by id so you maintain the subnet list in one place.
resource "unifi_network_firewall_group" "trusted_lans" {
  name = "Trusted LANs"
  type = "address-group"
  members = [
    "192.0.2.0/24", # Main (see guest.tf)
  ]
}

# Drop new connections originating on the IoT VLAN that are headed for a trusted
# LAN. Matching on connection state (new only) keeps Main→IoT replies working.
resource "unifi_network_firewall_rule" "iot_to_lan_drop" {
  name       = "Drop IoT to LAN"
  ruleset    = "LAN_IN"
  rule_index = 20010
  action     = "drop"
  protocol   = "all"

  # Source: anything on the IoT network, new connections only.
  src_network_id   = unifi_network_network.iot.id
  src_network_type = "NETv4"
  state_new        = true

  # Destination: any of the trusted LAN subnets.
  dst_firewall_group_ids = [unifi_network_firewall_group.trusted_lans.id]

  logging = true
}
