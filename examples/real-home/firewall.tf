# Simplified posture: IoT is fully isolated (intra-IoT only). Trusted reaches
# everything (no rules); guest is internet-only via purpose=guest (networks.tf).

resource "unifi_network_firewall_rule" "iot_to_trusted" {
  name             = "Block IoT to trusted"
  ruleset          = "LAN_IN"
  rule_index       = 20000
  action           = "drop"
  protocol         = "all"
  src_network_id   = unifi_network_network.iot.id
  src_network_type = "NETv4"
  dst_network_id   = unifi_network_network.trusted.id
  dst_network_type = "NETv4"
}

resource "unifi_network_firewall_rule" "iot_to_guest" {
  name             = "Block IoT to guest"
  ruleset          = "LAN_IN"
  rule_index       = 20001
  action           = "drop"
  protocol         = "all"
  src_network_id   = unifi_network_network.iot.id
  src_network_type = "NETv4"
  dst_network_id   = unifi_network_network.guest.id
  dst_network_type = "NETv4"
}

# No internet for IoT either — drop everything egressing the WAN.
resource "unifi_network_firewall_rule" "iot_to_wan" {
  name             = "Block IoT to internet"
  ruleset          = "WAN_OUT"
  rule_index       = 20010
  action           = "drop"
  protocol         = "all"
  src_network_id   = unifi_network_network.iot.id
  src_network_type = "NETv4"
}
