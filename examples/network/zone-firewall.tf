# Zone-based firewall use case: on a site running the zone-based firewall, stop
# the IoT network from opening connections to the main LAN, and keep cameras off
# the internet. (A classic-firewall site uses firewall.tf instead.)

# Built-in zones, looked up by key so the config carries no controller ids.
data "unifi_network_firewall_zone" "internal" {
  zone_key = "internal"
}

data "unifi_network_firewall_zone" "external" {
  zone_key = "external"
}

# New connections from IoT to Main are blocked; Main can still reach IoT, and
# replies flow because only the NEW state matches.
resource "unifi_network_firewall_policy" "iot_to_main" {
  name                  = "Block IoT to Main"
  action                = "BLOCK"
  connection_state_type = "CUSTOM"
  connection_states     = ["NEW"]

  source = {
    zone_id         = data.unifi_network_firewall_zone.internal.id
    matching_target = "NETWORK"
    network_ids     = [unifi_network_network.iot.id] # see guest.tf
  }
  destination = {
    zone_id         = data.unifi_network_firewall_zone.internal.id
    matching_target = "NETWORK"
    network_ids     = [unifi_network_network.main.id]
  }
}

# Cameras, kept in a firewall group, may not reach the internet.
resource "unifi_network_firewall_group" "cameras" {
  name    = "Cameras"
  type    = "address-group"
  members = ["203.0.113.20", "203.0.113.21"] # on the IoT network
}

resource "unifi_network_firewall_policy" "cameras_offline" {
  name    = "Cameras stay local"
  action  = "BLOCK"
  logging = true

  source = {
    zone_id              = data.unifi_network_firewall_zone.internal.id
    matching_target      = "IP"
    matching_target_type = "OBJECT"
    ip_group_id          = unifi_network_firewall_group.cameras.id
  }
  destination = {
    zone_id         = data.unifi_network_firewall_zone.external.id
    matching_target = "ANY"
  }
}

# A policy created in the UniFi UI, adopted as is. Import by name (or by _id).
import {
  to = unifi_network_firewall_policy.https_to_nas
  id = "name=Allow HTTPS to NAS"
}

resource "unifi_network_firewall_policy" "https_to_nas" {
  name     = "Allow HTTPS to NAS"
  action   = "ALLOW"
  protocol = "tcp"

  source = {
    zone_id         = data.unifi_network_firewall_zone.external.id
    matching_target = "ANY"
  }
  destination = {
    zone_id         = data.unifi_network_firewall_zone.internal.id
    matching_target = "IP"
    ips             = ["192.0.2.20"]
    port            = "443"
  }
}
