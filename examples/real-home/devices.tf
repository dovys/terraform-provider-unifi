# Gateway, two switches, two APs. Each switch port maps to its access VLAN via
# native_network_id; one port is MAC-locked for the outdoor camera.

resource "unifi_network_device" "gateway" {
  lifecycle {
    prevent_destroy = true
  }
  mac  = "aa:bb:cc:00:00:01"
  name = "Gateway"
}

resource "unifi_network_device" "core_switch" {
  lifecycle {
    prevent_destroy = true
  }
  mac  = "aa:bb:cc:00:00:02"
  name = "Core Switch"
  # Access ports on trusted, except 7-8 which serve iot devices.
  port_overrides = {
    1 = { native_network_id = unifi_network_network.trusted.id }
    2 = { native_network_id = unifi_network_network.trusted.id }
    3 = { native_network_id = unifi_network_network.trusted.id }
    4 = { native_network_id = unifi_network_network.trusted.id }
    5 = { native_network_id = unifi_network_network.trusted.id }
    6 = { native_network_id = unifi_network_network.trusted.id }
    7 = { native_network_id = unifi_network_network.iot.id }
    8 = { native_network_id = unifi_network_network.iot.id }
  }
}

resource "unifi_network_device" "iot_switch" {
  lifecycle {
    prevent_destroy = true
  }
  mac          = "aa:bb:cc:00:00:03"
  name         = "IoT Switch"
  led_override = "on"
  # Every access port on iot; port 8 is the outdoor camera (MAC-locked).
  port_overrides = {
    1 = { native_network_id = unifi_network_network.iot.id }
    2 = { native_network_id = unifi_network_network.iot.id }
    3 = { native_network_id = unifi_network_network.iot.id }
    4 = { native_network_id = unifi_network_network.iot.id }
    5 = { native_network_id = unifi_network_network.iot.id }
    6 = { native_network_id = unifi_network_network.iot.id }
    7 = { native_network_id = unifi_network_network.iot.id }
    8 = { native_network_id = unifi_network_network.iot.id, port_security_enabled = true, port_security_mac_addresses = ["aa:bb:cc:00:00:10"] }
  }
}

resource "unifi_network_device" "ap_living_room" {
  lifecycle {
    prevent_destroy = true
  }
  mac          = "aa:bb:cc:00:00:04"
  name         = "AP Living Room"
  led_override = "off"
}

resource "unifi_network_device" "ap_office" {
  lifecycle {
    prevent_destroy = true
  }
  mac          = "aa:bb:cc:00:00:05"
  name         = "AP Office"
  led_override = "off"
}
