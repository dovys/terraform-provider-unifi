# Static-IP WAN — an ISP-assigned fixed address.
#
# WAN uplinks are fixed in hardware: IMPORT the existing WAN and manage its
# config rather than creating it (Terraform 1.5+).
#
# The import `id` is the WAN's controller object id (`_id`, a 24-char hex
# string): run the example in ../data and read its `network_ids` output (a
# name => id map), or open Settings → Internet → the WAN in the UniFi app and
# copy the trailing segment of the browser URL (…/settings/internet/<id>).
#
# wan-dhcp.tf / wan-pppoe.tf / wan-static.tf are alternatives for the same
# primary uplink — apply the one that matches your ISP.

import {
  to = unifi_network_wan.primary
  id = "661f3a9c2b4d5e0012ab34ce"
}

resource "unifi_network_wan" "primary" {
  name          = "WAN"
  wan_type      = "static"
  network_group = "WAN"
  enabled       = true

  ip      = "203.0.113.2"
  netmask = "255.255.255.0"
  gateway = "203.0.113.1"

  dns_servers = ["1.1.1.1", "8.8.8.8"]
}
