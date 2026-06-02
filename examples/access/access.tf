# UniFi Access — over the one console session (no api_key). This phase ships the
# read-only door-group data source; the managed unifi_access_door_group resource
# is implemented but not yet registered (its write surface is reverse-engineered
# and pending live verification — see TODO.md).

# Reference an existing door group by name (read-only lookup).
data "unifi_access_door_group" "lobby" {
  name = "Lobby Doors"
}

output "lobby_door_ids" {
  value = data.unifi_access_door_group.lobby.door_ids
}

output "lobby_door_count" {
  value = data.unifi_access_door_group.lobby.door_count
}

# Once the managed resource is registered (after live verification), a door group
# is created like so:
#
# resource "unifi_access_door_group" "staff" {
#   name     = "Staff Entrances"
#   door_ids = [data.unifi_access_door_group.lobby.door_ids[0]]
# }
