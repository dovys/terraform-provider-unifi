# Uplinks are fixed in hardware — import, don't create. The id is the WAN's
# controller _id (see the ../data example's network_ids output).
import {
  to = unifi_network_wan.primary
  id = "DDDDDDDDEEEEEEEEFFFFFFFF"
}

resource "unifi_network_wan" "primary" {
  lifecycle {
    prevent_destroy = true
  }
  name          = "Fiber"
  wan_type      = "dhcp"
  network_group = "WAN"
  enabled       = true

  dns_servers       = ["1.1.1.1", "8.8.8.8"]
  download_kilobits = 1000000 # 1 Gbit symmetric fibre
  upload_kilobits   = 1000000
}

import {
  to = unifi_network_wan.backup
  id = "GGGGGGGGHHHHHHHHIIIIIIII"
}

# WAN2 can't be deleted, only disabled — kept as the "deliberately off" intent.
resource "unifi_network_wan" "backup" {
  name          = "WAN2"
  wan_type      = "dhcp"
  network_group = "WAN2"
  enabled       = false
}
