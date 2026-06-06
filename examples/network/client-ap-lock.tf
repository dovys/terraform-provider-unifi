# Client AP-lock use case: pin a finicky wireless client to one access point so
# the controller never roams or band-steers it away.
#
# Like a DHCP reservation, the lock is find-or-create by MAC: if the controller
# already tracks this client from a lease, the lock attaches to that record. The
# lock and a reservation can target the same MAC — the controller merges the two
# writes server-side. `terraform destroy` clears the lock (fixed_ap_enabled =
# false); the client record itself is left alone.
#
# Import an existing lock by MAC:
#
#   terraform import unifi_network_client_ap_lock.printer aa:bb:cc:dd:ee:02
resource "unifi_network_client_ap_lock" "printer" {
  mac    = "aa:bb:cc:dd:ee:02"
  ap_mac = "11:22:33:44:55:66" # MAC of the access point to pin to
}
