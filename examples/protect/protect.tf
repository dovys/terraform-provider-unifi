# UniFi Protect — configure existing, adopted devices over the one console
# session (no api_key). Cameras and the NVR are configure-existing, so import
# the live objects rather than creating them.

# The console-wide NVR settings (a singleton).
import {
  to = unifi_protect_nvr_settings.this
  id = "nvr" # any value; the singleton is resolved on import
}

resource "unifi_protect_nvr_settings" "this" {
  name             = "HQ Console"
  timezone         = "America/New_York"
  time_format      = "24h"
  temperature_unit = "F"
  ssh_enabled      = false
}

# A doorbell camera, imported by MAC and then configured.
import {
  to = unifi_protect_camera.front_door
  id = "28:70:4e:11:22:33"
}

resource "unifi_protect_camera" "front_door" {
  mac = "28:70:4e:11:22:33"

  name                 = "Front Door"
  mic_enabled          = true
  mic_volume           = 80
  status_light_enabled = true

  video_mode     = "default"
  recording_mode = "detections"

  osd_name_enabled = true
  osd_date_enabled = true

  smart_detect_object_types = ["person", "package"]

  # Doorbell-only LCD message.
  lcd_message_type = "DO_NOT_DISTURB"

  # Cameras are expensive hardware: destroy reverts to state removal and keeps
  # the device adopted. Guard against accidental unadopt.
  lifecycle {
    prevent_destroy = true
  }
}

# Reference a camera Terraform does not manage (read-only lookup).
data "unifi_protect_camera" "garage" {
  mac = "28:70:4e:44:55:66"
}

output "garage_state" {
  value = data.unifi_protect_camera.garage.state
}

# A floodlight, imported by MAC and configured.
import {
  to = unifi_protect_light.driveway
  id = "28:70:4e:aa:bb:cc"
}

resource "unifi_protect_light" "driveway" {
  mac = "28:70:4e:aa:bb:cc"

  name              = "Driveway Flood"
  mode              = "motion"
  enable_at         = "dark"
  led_level         = 4
  pir_sensitivity   = 80
  pir_duration      = 15000 # ms
  lux_sensitivity   = "medium"
  indicator_enabled = true
}

# A UP-Sense multi-sensor.
import {
  to = unifi_protect_sensor.garage
  id = "28:70:4e:dd:ee:ff"
}

resource "unifi_protect_sensor" "garage" {
  mac        = "28:70:4e:dd:ee:ff"
  name       = "Garage Sensor"
  mount_type = "garage"

  motion_enabled      = true
  motion_sensitivity  = 60
  temperature_enabled = true
  temperature_low     = 4.0
  temperature_high    = 35.0
  led_enabled         = true
}

# A chime that rings for the front-door doorbell.
import {
  to = unifi_protect_chime.hallway
  id = "28:70:4e:11:99:88"
}

resource "unifi_protect_chime" "hallway" {
  mac          = "28:70:4e:11:99:88"
  name         = "Hallway Chime"
  volume       = 70
  repeat_times = 2
  camera_ids   = [unifi_protect_camera.front_door.id]
}

# A saved multi-camera live view (created, not imported).
resource "unifi_protect_liveview" "lobby" {
  name      = "Lobby Grid"
  layout    = 4
  is_global = true

  slots = [
    {
      cameras = [unifi_protect_camera.front_door.id]
    },
    {
      cameras        = [data.unifi_protect_camera.garage.id]
      cycle_interval = 10
    },
  ]
}

# A viewport that displays the live view above.
import {
  to = unifi_protect_viewer.lobby_tv
  id = "28:70:4e:55:44:33"
}

resource "unifi_protect_viewer" "lobby_tv" {
  mac      = "28:70:4e:55:44:33"
  name     = "Lobby TV"
  liveview = unifi_protect_liveview.lobby.id
}
