# Wi-Fi use case: a WPA2/WPA3 home SSID on the main LAN and an isolated guest
# SSID on the guest VLAN — both bound to the networks defined in guest.tf.
#
# Keep the passphrase out of version control: read it from a variable (set via
# TF_VAR_wifi_passphrase or a .tfvars file you don't commit) rather than
# hard-coding it here.
variable "wifi_passphrase" {
  type      = string
  sensitive = true
}

variable "guest_passphrase" {
  type      = string
  sensitive = true
}

# Main home Wi-Fi: WPA2/WPA3 transition mode, on the untagged main LAN, 2.4 +
# 5 GHz. Protected Management Frames are required for WPA3.
resource "unifi_network_wlan" "home" {
  name       = "Home"
  network_id = unifi_network_network.main.id

  security        = "wpapsk"
  passphrase      = var.wifi_passphrase
  wpa3_support    = true
  wpa3_transition = true
  pmf_mode        = "optional"

  bands = ["2g", "5g"]
}

# Guest Wi-Fi: WPA2-only on the guest VLAN, with client-to-client isolation and
# the guest policy applied. 5 GHz only to keep the band clear for the home SSID.
resource "unifi_network_wlan" "guest" {
  name       = "Guest"
  network_id = unifi_network_network.guest.id

  security   = "wpapsk"
  passphrase = var.guest_passphrase

  is_guest     = true
  l2_isolation = true

  bands = ["5g"]
}

# Optional: broadcast the home SSID only on a named subset of APs. The default
# AP group (every adopted AP) is controller-managed and protected — create a new
# group when you want to limit coverage, then reference it from the SSID's
# `ap_group_ids`.
#
# resource "unifi_network_ap_group" "downstairs" {
#   name        = "Downstairs"
#   device_macs = ["aa:bb:cc:00:11:22", "aa:bb:cc:00:11:33"]
# }
