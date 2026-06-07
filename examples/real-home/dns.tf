# Local A record so the console name resolves to the gateway on-LAN; pairs with
# the cert in console.tf.
resource "unifi_network_dns_record" "console" {
  name    = "console.home.example"
  type    = "A"
  value   = "192.168.30.1"
  enabled = true
}
