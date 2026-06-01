# Guest network use case: a trusted main LAN plus an isolated guest network on
# its own VLAN, each with its own DHCP scope.
#
# Wi-Fi SSIDs attach to these networks with a future `unifi_network_wlan`
# resource (a later phase) — e.g. a "Home" SSID on unifi_network_network.main
# and a captive-portal "Guest" SSID on unifi_network_network.guest. Today this
# models the networks and VLANs the SSIDs will bind to.

# The main LAN usually already exists as the controller's built-in "Default"
# network — import it rather than creating a duplicate (Terraform 1.5+; see
# https://spacelift.io/blog/terraform-import-block). The guest and cameras VLANs
# below are new, so they're created normally. Replace the id with your
# controller's networkconf `_id` (find it with the data sources in ../data).
import {
  to = unifi_network_network.main
  id = "661f3a9c2b4d5e0012ab34cd"
}

# Primary routed LAN, untagged (the default VLAN).
resource "unifi_network_network" "main" {
  name    = "Main"
  purpose = "corporate"
  subnet  = "192.168.1.1/24"

  dhcp_enabled = true
  dhcp_start   = "192.168.1.100"
  dhcp_stop    = "192.168.1.254"
  dhcp_dns     = ["1.1.1.1", "1.0.0.1"]

  igmp_snooping = true
  mdns_enabled  = true
}

# Guest network on VLAN 40: internet-only, isolated from the main LAN, with its
# own DHCP scope. Pair it with a guest SSID and (later) a guest firewall policy.
resource "unifi_network_network" "guest" {
  name    = "Guest"
  purpose = "guest"
  vlan_id = 40
  subnet  = "192.168.40.1/24"

  dhcp_enabled = true
  dhcp_start   = "192.168.40.100"
  dhcp_stop    = "192.168.40.254"
  dhcp_dns     = ["1.1.1.1"]

  internet_access_enabled = true
}

# A layer-2-only VLAN (no gateway, no DHCP) — e.g. for an isolated camera or
# IoT SSID bridged elsewhere.
resource "unifi_network_network" "cameras" {
  name    = "Cameras"
  purpose = "vlan-only"
  vlan_id = 50
}
