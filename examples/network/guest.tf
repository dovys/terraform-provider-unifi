# Smart-home network use case: a trusted main LAN, an isolated guest network, and
# an isolated IoT network — each on its own VLAN with its own DHCP scope.
#
# Wi-Fi SSIDs attach to these networks via `unifi_network_wlan` (see wifi.tf in
# this directory): "MySmartHome" on unifi_network_network.main, "MySmartHome
# Guest" on unifi_network_network.guest, and "MySmartHome IoT" on
# unifi_network_network.iot. This file models the networks and VLANs the SSIDs
# bind to.

# The main LAN usually already exists as the controller's built-in "Default"
# network — import it rather than creating a duplicate (Terraform 1.5+; see
# https://spacelift.io/blog/terraform-import-block). The guest and IoT VLANs
# below are new, so they're created normally.
#
# The import `id` is the network's controller object id (`_id`, a 24-char hex
# string). The UniFi UI doesn't show it — discover it by running the example in
# ../data and reading its `network_ids` output (a name => id map), then copy the
# value for "Default".
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

# IoT network on VLAN 50: its own routed subnet and DHCP scope for smart-home
# gadgets, internet-only and isolated from the main LAN. The IoT SSID binds here.
resource "unifi_network_network" "iot" {
  name    = "IoT"
  purpose = "corporate"
  vlan_id = 50
  subnet  = "192.168.50.1/24"

  dhcp_enabled = true
  dhcp_start   = "192.168.50.100"
  dhcp_stop    = "192.168.50.254"
  dhcp_dns     = ["1.1.1.1"]

  igmp_snooping = true # many IoT devices rely on mDNS/multicast discovery
  mdns_enabled  = true
}
