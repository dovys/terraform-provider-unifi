# Local DNS use cases: records the console resolves for clients on the network.

# An A record for a NAS on the IoT VLAN.
resource "unifi_network_dns_record" "nas" {
  name  = "nas.lan"
  type  = "A"
  value = "203.0.113.10"
}

# A CNAME alias pointing at it.
resource "unifi_network_dns_record" "files" {
  name  = "files.lan"
  type  = "CNAME"
  value = "nas.lan"
}

# An SRV record uses the extra port/priority/weight fields.
resource "unifi_network_dns_record" "sip" {
  name     = "_sip._tcp.lan"
  type     = "SRV"
  value    = "sip.lan"
  port     = 5060
  priority = 10
  weight   = 5
}
