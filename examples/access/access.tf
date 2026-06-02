# UniFi Access — over the one console session (no api_key). The door-group write
# surface is reverse-engineered and HIL-unverified (see TODO.md).

# Reference an existing door group by name (read-only lookup).
data "unifi_access_door_group" "lobby" {
  name = "Lobby Doors"
}

output "lobby_door_ids" {
  value = data.unifi_access_door_group.lobby.door_ids
}

# Manage a door group: a named set of doors used to scope access policies.
resource "unifi_access_door_group" "staff" {
  name = "Staff Entrances"
  door_ids = [
    data.unifi_access_door_group.lobby.door_ids[0],
  ]
}
