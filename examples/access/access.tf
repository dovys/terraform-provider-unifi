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

# Manage an access policy: grant a set of resources (doors / door groups), in
# effect on an optional schedule. The subject/rule graph is controller-managed.
resource "unifi_access_policy" "staff_hours" {
  name    = "Staff — business hours"
  enabled = true
  # schedule_id = "<schedule unique_id>"  # optional: gate the policy to a schedule

  resources {
    type = "door_group"
    id   = unifi_access_door_group.staff.id
  }
}

# Reference an existing policy by name (read-only).
data "unifi_access_policy" "contractors" {
  name = "Contractors"
}

output "contractor_door_count" {
  value = data.unifi_access_policy.contractors.door_count
}

# Manage a holiday group: a named set of holidays used to scope unlock schedules.
# Membership references existing holiday unique_ids.
resource "unifi_access_holiday_group" "us" {
  name        = "US Federal Holidays"
  description = "Days the office is closed"
  holiday_ids = [
    # "<holiday unique_id>", ...
  ]
}

# Look up a discovered door by name.
data "unifi_access_door" "front" {
  name = "Front Entrance"
}

# Configure an existing (discovered) door. Update/import-only: destroy stops
# managing it without deleting the door.
resource "unifi_access_door" "front" {
  door_id    = data.unifi_access_door.front.id
  name       = "Front Entrance"
  floor_name = "Ground Floor"
}

# Set the guard user set of a door (the users authorized to act as guards for
# it). Update/import-only, keyed by door_id; set user_ids = [] to clear, and
# destroy stops managing the set without changing it. user_ids reference
# existing Access users (the provider does not model users).
resource "unifi_access_door_guard" "front" {
  door_id = data.unifi_access_door.front.id
  user_ids = [
    # "<access user unique_id>", ...
  ]
}
