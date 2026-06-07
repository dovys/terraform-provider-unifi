# Fixed IPs by MAC (find-or-create). Import an existing one:
#   terraform import unifi_network_dhcp_reservation.nas aa:bb:cc:00:01:10

# --- trusted: things you administer or print to ---
resource "unifi_network_dhcp_reservation" "nas" {
  mac        = "aa:bb:cc:00:01:10"
  ip_address = "192.168.30.10"
  name       = "NAS"
}

resource "unifi_network_dhcp_reservation" "home_assistant" {
  mac        = "aa:bb:cc:00:01:11"
  ip_address = "192.168.30.11"
  name       = "Home Assistant"
}

resource "unifi_network_dhcp_reservation" "printer" {
  mac        = "aa:bb:cc:00:01:12"
  ip_address = "192.168.30.12"
  name       = "Office Printer"
}

# --- iot: appliances and media, kept on the isolated VLAN ---
resource "unifi_network_dhcp_reservation" "thermostat" {
  mac        = "aa:bb:cc:00:01:20"
  ip_address = "192.168.40.20"
  name       = "Thermostat"
}

resource "unifi_network_dhcp_reservation" "robot_vacuum" {
  mac        = "aa:bb:cc:00:01:21"
  ip_address = "192.168.40.21"
  name       = "Robot Vacuum"
}

resource "unifi_network_dhcp_reservation" "media_streamer" {
  mac        = "aa:bb:cc:00:01:22"
  ip_address = "192.168.40.22"
  name       = "Media Streamer"
}

resource "unifi_network_dhcp_reservation" "threed_printer" {
  mac        = "aa:bb:cc:00:01:23"
  ip_address = "192.168.40.23"
  name       = "3D Printer"
}
