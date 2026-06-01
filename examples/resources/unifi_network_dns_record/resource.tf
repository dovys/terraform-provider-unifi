# A local A record resolved by the console's DNS.
resource "unifi_network_dns_record" "nas" {
  name  = "nas.lan"
  type  = "A"
  value = "10.0.30.10"
}

# A CNAME alias.
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
