# Dynamic-DNS use case: keep a hostname pointed at the gateway's WAN IP on a
# residential connection where the ISP rotates the address. Source the provider
# credential from a variable — never hard-code it.
resource "unifi_network_dynamic_dns" "home" {
  service   = "dyndns"
  hostname  = "home.example.com"
  login     = "ddns-user"
  password  = var.ddns_password
  interface = "wan"
}

variable "ddns_password" {
  type      = string
  sensitive = true
}
