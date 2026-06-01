# Routing & forwarding use case: reach a downstream network behind another
# router, and expose an internal service to the internet.

# Static route: send traffic for a lab subnet (192.168.50.0/24) that lives
# behind a secondary router at 10.0.0.254 on the main LAN.
resource "unifi_network_static_route" "lab" {
  name       = "to-lab"
  network    = "192.168.50.0/24"
  route_type = "nexthop-route"
  next_hop   = "10.0.0.254"
  distance   = 1
}

# Port forward: expose the NAS's HTTPS UI on the WAN. Restrict the source to a
# known remote office rather than leaving it open to the whole internet.
resource "unifi_network_port_forward" "nas_https" {
  name             = "NAS HTTPS"
  interface        = "wan"
  protocol         = "tcp"
  source           = "203.0.113.0/24"
  destination_port = "8443"
  forward_ip       = "192.168.1.10"
  forward_port     = "443"
}
