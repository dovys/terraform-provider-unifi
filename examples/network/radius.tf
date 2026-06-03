# RADIUS use case: 802.1X / WPA-Enterprise authentication. Two shapes —
#   1. the gateway's built-in RADIUS server with local user accounts, and
#   2. a profile pointing at an external RADIUS server (e.g. a FreeRADIUS box).
#
# A WPA-Enterprise SSID or a wired 802.1X port profile references one of these
# profiles to authenticate clients.

# 1) Built-in server: let the gateway be the RADIUS server and define users
#    locally. The account's tunnel attributes hand the client a dynamic VLAN
#    (Tunnel-Type 13 = VLAN, Tunnel-Medium-Type 6 = IEEE-802) on auth.
resource "unifi_network_radius_profile" "builtin" {
  name                = "Built-in RADIUS"
  use_usg_auth_server = true
  use_usg_acct_server = true

  # Honor the VLAN the server assigns per user.
  vlan_enabled   = true
  vlan_wlan_mode = "optional"
}

resource "unifi_network_radius_account" "alice" {
  name     = "alice"
  password = var.radius_alice_password

  # Put alice on VLAN 30 when she authenticates.
  tunnel_type        = 13
  tunnel_medium_type = 6
  vlan               = 30
}

# 2) External server: forward auth/accounting to a dedicated RADIUS host. The
#    shared secret is sensitive — source it from a variable, never hard-code it.
resource "unifi_network_radius_profile" "corp" {
  name = "Corp RADIUS"

  auth_servers = [
    {
      ip       = "192.0.2.9"
      port     = 1812
      x_secret = var.radius_corp_secret
    },
  ]

  acct_servers = [
    {
      ip       = "192.0.2.9"
      port     = 1813
      x_secret = var.radius_corp_secret
    },
  ]

  interim_update_enabled  = true
  interim_update_interval = 3600
}

# WPA-Enterprise (802.1X) SSID: authenticate each client against a RADIUS profile
# instead of a shared passphrase. `radius_profile_id` is the binding that makes
# the profile referenceable from the SSID; WPA3-Enterprise requires PMF. Bound to
# the main LAN defined in guest.tf.
resource "unifi_network_wlan" "corp" {
  name       = "MyCorp Secure"
  network_id = unifi_network_network.main.id

  security          = "wpaeap"
  radius_profile_id = unifi_network_radius_profile.corp.id
  pmf_mode          = "required"

  bands = ["2g", "5g"]
}

variable "radius_alice_password" {
  type      = string
  sensitive = true
}

variable "radius_corp_secret" {
  type      = string
  sensitive = true
}
