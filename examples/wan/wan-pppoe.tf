# PPPoE WAN — many DSL/fibre ISPs. `pppoe_password` is write-only (sensitive,
# never read back).
#
# WAN uplinks are fixed in hardware: IMPORT the existing WAN and manage its
# config rather than creating it. Replace the id with your controller's
# networkconf `_id` (find it with the data sources in ../data).
#
# wan-dhcp.tf / wan-pppoe.tf / wan-static.tf are alternatives for the same
# primary uplink — apply the one that matches your ISP.

import {
  to = unifi_network_wan.primary
  id = "661f3a9c2b4d5e0012ab34ce"
}

resource "unifi_network_wan" "primary" {
  name          = "WAN"
  wan_type      = "pppoe"
  network_group = "WAN"
  enabled       = true

  pppoe_username = "user@isp.example"
  pppoe_password = var.isp_password # sensitive

  dns_servers = ["1.1.1.1", "8.8.8.8"]
}
