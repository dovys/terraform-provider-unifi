# User-group use case: bandwidth/QoS groups you assign to clients to cap their
# up/down rate. A classic shape is throttling guests so they can't saturate the
# uplink while leaving trusted clients unlimited.
#
# Rates are in kbps. Omit a cap (or set it to 0) for "unlimited" on that
# direction. A group is applied to a client by reference from the client's
# settings; defining it here keeps the rate policy in one place.

# Throttle guests to 20 Mbit down / 5 Mbit up.
resource "unifi_network_user_group" "guests" {
  name              = "Guests"
  qos_rate_max_down = 20000
  qos_rate_max_up   = 5000
}

# Cap IoT devices hard — they should never need much, and a runaway device
# can't then hog the link. Down only; uploads left unlimited.
resource "unifi_network_user_group" "iot" {
  name              = "IoT"
  qos_rate_max_down = 10000
}

# The controller ships a protected "Default" group (attr_no_delete) that can't be
# removed. Import it to manage its caps rather than recreating it; `terraform
# destroy` drops it from state instead of attempting a delete the controller
# rejects. Find its id under Settings → Profiles → User Groups (or via the API):
#
#   terraform import unifi_network_user_group.default <user-group-id>
import {
  to = unifi_network_user_group.default
  id = "000000000000000000000000" # replace with the Default group's _id
}

resource "unifi_network_user_group" "default" {
  name = "Default"
  # No caps: the built-in default is unlimited.
}
