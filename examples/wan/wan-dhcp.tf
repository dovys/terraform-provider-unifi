# DHCP WAN — the common case (cable/fibre that hands you an address).
#
# WAN uplinks are fixed in hardware: IMPORT the existing WAN/WAN2 and manage
# their config, don't create them. The `import` blocks bring them into state on
# the next `terraform apply` (Terraform 1.5+; see
# https://spacelift.io/blog/terraform-import-block).
#
# Each import `id` is the WAN's controller object id (`_id`, a 24-char hex
# string). The UniFi UI doesn't show it, so discover it with the data sources:
# run the example in ../data and read its `network_ids` output — a name => id
# map of every network (WANs included) — and copy the value for your WAN. (For
# scripting, GET /proxy/network/api/s/<site>/rest/networkconf returns each
# object's `_id` too.)
#
# wan-dhcp.tf / wan-pppoe.tf / wan-static.tf are alternatives for the same
# primary uplink — apply the one that matches your ISP.

import {
  to = unifi_network_wan.primary
  id = "661f3a9c2b4d5e0012ab34ce"
}

resource "unifi_network_wan" "primary" {
  name          = "WAN"
  wan_type      = "dhcp"
  network_group = "WAN"
  enabled       = true

  # Override DNS (leave empty for automatic/ISP-provided).
  dns_servers = ["1.1.1.1", "8.8.8.8"]

  # Advertised ISP bandwidth (kbit/s) — used for smart-queue / load balancing.
  upload_kilobits   = 40000
  download_kilobits = 1000000
}

import {
  to = unifi_network_wan.backup
  id = "661f3a9c2b4d5e0012ab34d0"
}

# A second uplink (WAN2) configured as failover only.
resource "unifi_network_wan" "backup" {
  name              = "WAN2"
  wan_type          = "dhcp"
  network_group     = "WAN2"
  load_balance_type = "failover-only"
}
