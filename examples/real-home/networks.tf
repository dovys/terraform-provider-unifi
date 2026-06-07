# VLANs. Minimal intent: identity + DHCP pool + any non-default override. Three
# segments: trusted (reaches all), iot (locked down), guest (internet-only).

resource "unifi_network_network" "trusted" {
  lifecycle {
    prevent_destroy = true
  }
  name       = "trusted"
  vlan_id    = 30
  subnet     = "192.168.30.1/24"
  dhcp_start = "192.168.30.50"
  dhcp_stop  = "192.168.30.250"
}

resource "unifi_network_network" "iot" {
  lifecycle {
    prevent_destroy = true
  }
  name       = "iot"
  vlan_id    = 40
  subnet     = "192.168.40.1/24"
  dhcp_start = "192.168.40.50"
  dhcp_stop  = "192.168.40.250"
}

# purpose=guest: internet-only, isolated from every other VLAN.
resource "unifi_network_network" "guest" {
  lifecycle {
    prevent_destroy = true
  }
  name       = "guest"
  purpose    = "guest"
  vlan_id    = 50
  subnet     = "192.168.50.1/24"
  dhcp_start = "192.168.50.50"
  dhcp_stop  = "192.168.50.250"
  dhcp_dns   = ["1.1.1.1", "9.9.9.9"]
}

# Factory-default "Default" LAN — imported and kept unused. Find its _id via the
# ../data example's network_ids output.
import {
  to = unifi_network_network.default_unused
  id = "AAAAAAAABBBBBBBBCCCCCCCC"
}

resource "unifi_network_network" "default_unused" {
  name = "Default"
}
