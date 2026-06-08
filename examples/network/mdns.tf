# mDNS reflector scope. `unifi_network_mdns` is a site-level singleton: it sets
# how widely the controller bridges Bonjour/mDNS service discovery (AirPlay,
# Chromecast, printers, …) across your networks (VLANs).
#
# Choose a mode:
#   all  — reflect across every network
#   none — reflector off
#   some — reflect only across the networks in network_ids (used here)

resource "unifi_network_network" "iot" {
  name    = "IoT"
  purpose = "corporate"
  vlan_id = 50
  subnet  = "192.0.2.1/24"
}

resource "unifi_network_mdns" "reflector" {
  mode = "some"
  network_ids = [
    unifi_network_network.iot.id,
    # add other unifi_network_network.<name>.id you want bridged
  ]
}
