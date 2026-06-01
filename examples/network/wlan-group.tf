# WLAN-group use case: WLAN groups are the older organizational unit for SSIDs
# (AP groups largely supersede them). They're mostly system-managed — the common
# operation is importing the controller's protected default group to manage its
# name, rather than creating new ones. `terraform destroy` drops the protected
# default from state instead of attempting a delete the controller rejects.
#
#   terraform import unifi_network_wlan_group.default <wlan-group-id>
import {
  to = unifi_network_wlan_group.default
  id = "000000000000000000000000" # replace with the Default group's _id
}

resource "unifi_network_wlan_group" "default" {
  name = "Default"
}
