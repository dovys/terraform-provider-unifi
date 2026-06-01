# Device management use case: adopt-if-needed, then configure a switch — its
# name, status LED, and a couple of ports. One port is pinned to the IoT access
# profile from port-profile.tf; another has PoE cut for a non-powered device.
#
# The device must already be visible to the controller (adopted, or pending
# adoption — i.e. broadcasting). Terraform adopts it if needed, then applies the
# config below. It manages ONLY the ports listed in `port_overrides`; every other
# port is left exactly as the controller has it, and removing a port from this map
# reverts that port to its defaults.
#
# To bring an already-adopted device under management without re-adopting, import
# it by MAC (or by its controller `_id`):
#   terraform import unifi_network_device.garage_switch 00:11:22:33:44:55
resource "unifi_network_device" "garage_switch" {
  mac          = "00:11:22:33:44:55" # the switch's MAC (shown in the UniFi UI)
  name         = "Garage Switch"
  led_override = "off"

  port_overrides = {
    # Port 5: pin to the reusable IoT access profile (see port-profile.tf).
    "5" = {
      port_profile_id = unifi_network_port_profile.iot_access.id
    }
    # Port 6: a non-PoE appliance — cut power and label the port.
    "6" = {
      name     = "NVR"
      poe_mode = "off"
    }
  }

  # Safe by default: `terraform destroy` reverts the managed port overrides and
  # drops the device from state. Set this to true to fully forget (unadopt) it,
  # and guard production gear with a `prevent_destroy` lifecycle block.
  forget_on_destroy = false
}
