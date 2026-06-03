# DHCP reservation use case: pin the NAS to a fixed address on the main LAN and
# publish a local DNS name for it.
#
# Reservations are find-or-create by MAC: if the controller already tracks this
# client from a DHCP lease, the reservation attaches to that record. `terraform
# destroy` clears the reservation (the client record itself is left alone).
#
# Import an existing reservation by MAC, which the UI does show (Client →
# Settings → Fixed IP):
#
#   terraform import unifi_network_dhcp_reservation.nas aa:bb:cc:dd:ee:01
resource "unifi_network_dhcp_reservation" "nas" {
  mac        = "aa:bb:cc:dd:ee:01"
  ip_address = "192.0.2.10"
  network_id = unifi_network_network.main.id
  name       = "NAS"

  # Resolve nas.lan to the reserved address on the local network.
  local_dns_record         = "nas.lan"
  local_dns_record_enabled = true
}
