# Three SSIDs on the trusted/guest/iot VLANs. passphrase is write-only — set it
# (via TF_VAR_*) only to change it.

resource "unifi_network_wlan" "main" {
  name       = "HomeNet"
  network_id = unifi_network_network.trusted.id
  security   = "wpapsk"
  passphrase = var.wifi_main_passphrase
  bands      = ["5g", "6g"]

  # Allow-list — only known devices may associate. Privacy MACs go stale on
  # re-randomization.
  mac_filter_enabled = true
  mac_filter_policy  = "allow"
  mac_filter_list = [
    "02:00:00:00:00:01", # Laptop 1
    "02:00:00:00:00:02", # Laptop 2
    "02:00:00:00:00:03", # Laptop 3
  ]
}

resource "unifi_network_wlan" "guest" {
  name         = "HomeNet Guest"
  network_id   = unifi_network_network.guest.id
  security     = "wpapsk"
  passphrase   = var.wifi_guest_passphrase
  bands        = ["5g", "6g"]
  is_guest     = true
  l2_isolation = true
}

# WPA2 on 2.4 GHz only — most smart-home gadgets are 2.4 GHz.
resource "unifi_network_wlan" "iot" {
  name       = "HomeNet IoT"
  network_id = unifi_network_network.iot.id
  security   = "wpapsk"
  passphrase = var.wifi_iot_passphrase
  bands      = ["2g"]
}
