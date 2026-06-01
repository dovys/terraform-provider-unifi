# Switch-port profiles use case: reusable port configurations you assign to
# switch ports instead of hand-editing each one. Two common smart-home shapes:
# an access port pinned to the IoT VLAN, and a trunk uplink for an AP.
#
# A profile is applied to a port by reference (the device resource binds a port
# to a profile id); defining it here keeps the VLAN/PoE/STP policy in one place.

# Access profile: pin a port to the IoT VLAN so anything plugged in (a smart-home
# hub, a camera) lands on VLAN 50 untagged and can't reach other VLANs. PoE auto
# powers a device that needs it. References the IoT network from guest.tf.
resource "unifi_network_port_profile" "iot_access" {
  name = "IoT Access"

  # Untagged on the IoT network, no tagged VLANs allowed through.
  forward           = "native"
  native_network_id = unifi_network_network.iot.id
  tagged_vlan_mgmt  = "block_all"

  poe_mode = "auto"
}

# Trunk profile for an access-point uplink: native on the main LAN, carrying the
# guest and IoT VLANs tagged so SSIDs on those networks work. PoE powers the AP.
resource "unifi_network_port_profile" "ap_trunk" {
  name = "AP Trunk"

  # Native (untagged) on Main; allow all configured tagged VLANs through.
  forward           = "all"
  native_network_id = unifi_network_network.main.id
  tagged_vlan_mgmt  = "auto"

  poe_mode = "auto"

  # Keep STP on for an uplink port so a cabling loop can't take down the network.
  stp_port_mode  = true
  stp_bpdu_guard = false
}
