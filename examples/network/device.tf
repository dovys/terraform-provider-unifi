# Device management use case: adopt-if-needed, then configure a switch — its
# name, status LED, and a few ports. One port is pinned to the IoT access
# profile from port-profile.tf; one is moved to a different VLAN inline (no
# profile); another has PoE cut for a non-powered device.
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
    # Port 7: move a single port to the IoT VLAN inline — an access port
    # untagged on IoT, no tagged VLANs — without defining a reusable profile.
    # Use this for a one-off; reach for a port_profile when several ports share
    # the same policy. `forward` is computed by the controller from this intent
    # (native network + tagged_vlan_mgmt), so it isn't set here.
    "7" = {
      name              = "Thermostat"
      native_network_id = unifi_network_network.iot.id
      tagged_vlan_mgmt  = "block_all"
    }
    # Port 6: a non-PoE appliance — cut power and label the port.
    "6" = {
      name     = "NVR"
      poe_mode = "off"
    }
    # Port 8: administratively disabled — link down, no forwarding. The controller
    # has no off switch of its own; `disabled` writes the VLAN combination it
    # derives `forward = "disabled"` from. Mutually exclusive with the VLAN/profile
    # fields, since those would steer the same derivation the other way.
    "8" = {
      name     = "Unused"
      disabled = true
    }
  }

  # Safe by default: `terraform destroy` reverts the managed port overrides and
  # drops the device from state. Set this to true to fully forget (unadopt) it,
  # and guard production gear with a `prevent_destroy` lifecycle block.
  forget_on_destroy = false
}
