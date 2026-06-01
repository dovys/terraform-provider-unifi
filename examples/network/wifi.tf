# Wi-Fi use case: three SSIDs for a smart home, bound to the networks defined in
# guest.tf —
#   - "MySmartHome"       WPA3-only on the main LAN (2.4 + 5 GHz)
#   - "MySmartHome Guest" WPA2/WPA3 transition on the isolated guest VLAN
#   - "MySmartHome IoT"   WPA2 on 2.4 GHz only, on the isolated IoT VLAN
#
# Keep passphrases out of version control: read them from variables (set via
# TF_VAR_* or an uncommitted .tfvars file) rather than hard-coding them here.
variable "home_passphrase" {
  type      = string
  sensitive = true
}

variable "guest_passphrase" {
  type      = string
  sensitive = true
}

variable "iot_passphrase" {
  type      = string
  sensitive = true
}

# Main home Wi-Fi: WPA3-only (no WPA2 fallback), which requires Protected
# Management Frames. Modern phones and laptops support this; fall back to the
# guest/IoT SSIDs for older gear. Broadcasts on both bands.
resource "unifi_network_wlan" "home" {
  name       = "MySmartHome"
  network_id = unifi_network_network.main.id

  security        = "wpapsk"
  passphrase      = var.home_passphrase
  wpa3_support    = true
  wpa3_transition = false # WPA3-only
  pmf_mode        = "required"

  bands = ["2g", "5g"]
}

# Guest Wi-Fi: WPA2/WPA3 transition mode so older guest devices still connect,
# on the guest VLAN with client-to-client isolation and the guest policy.
resource "unifi_network_wlan" "guest" {
  name       = "MySmartHome Guest"
  network_id = unifi_network_network.guest.id

  security        = "wpapsk"
  passphrase      = var.guest_passphrase
  wpa3_support    = true
  wpa3_transition = true
  pmf_mode        = "optional"

  is_guest     = true
  l2_isolation = true

  bands = ["2g", "5g"]
}

# IoT Wi-Fi: plain WPA2 on 2.4 GHz only — most smart-home gadgets are 2.4 GHz and
# don't speak WPA3. Bound to the isolated IoT VLAN, with client isolation so a
# compromised gadget can't reach the rest of the network.
resource "unifi_network_wlan" "iot" {
  name       = "MySmartHome IoT"
  network_id = unifi_network_network.iot.id

  security   = "wpapsk"
  passphrase = var.iot_passphrase

  l2_isolation = true

  bands = ["2g"]
}

# Optional: broadcast an SSID only on a named subset of APs. The default AP group
# (every adopted AP) is controller-managed and protected — create a new group to
# limit coverage, then set the SSID's `ap_group_ids`.
#
# resource "unifi_network_ap_group" "downstairs" {
#   name        = "Downstairs"
#   device_macs = ["aa:bb:cc:00:11:22", "aa:bb:cc:00:11:33"]
# }
