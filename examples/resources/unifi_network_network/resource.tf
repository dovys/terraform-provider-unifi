# A routed VLAN with its own DHCP scope and custom DNS servers.
resource "unifi_network_network" "iot" {
  name    = "IoT"
  purpose = "corporate"
  vlan_id = 30
  subnet  = "10.0.30.1/24"

  dhcp_enabled = true
  dhcp_start   = "10.0.30.100"
  dhcp_stop    = "10.0.30.254"
  dhcp_dns     = ["1.1.1.1", "9.9.9.9"]

  domain_name = "iot.lan"
}

# A layer-2-only VLAN (no gateway, no DHCP) — e.g. for an isolated AP SSID.
resource "unifi_network_network" "cameras" {
  name    = "Cameras"
  purpose = "vlan-only"
  vlan_id = 40
}
