# A WAN uplink is usually fixed in hardware — import the existing WAN/WAN2 and
# manage its configuration rather than creating one.
resource "unifi_network_wan" "primary" {
  name          = "WAN"
  wan_type      = "dhcp"
  network_group = "WAN"
  enabled       = true

  # Override DNS (leave empty for automatic/ISP-provided).
  dns_servers = ["1.1.1.1", "8.8.8.8"]

  # Advertised ISP bandwidth (kbit/s) — used for smart-queue / load balancing.
  upload_kilobits   = 40000
  download_kilobits = 1000000
}

# A second uplink configured as failover only.
resource "unifi_network_wan" "backup" {
  name              = "WAN2"
  wan_type          = "dhcp"
  network_group     = "WAN2"
  load_balance_type = "failover-only"
}

# A PPPoE uplink (e.g. many DSL/fibre ISPs). pppoe_password is write-only.
resource "unifi_network_wan" "dsl" {
  name           = "WAN"
  wan_type       = "pppoe"
  pppoe_username = "user@isp.example"
  pppoe_password = var.isp_password # sensitive
}

# A static-IP uplink.
resource "unifi_network_wan" "static" {
  name     = "WAN"
  wan_type = "static"
  ip       = "203.0.113.2"
  netmask  = "255.255.255.0"
  gateway  = "203.0.113.1"
}
