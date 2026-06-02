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
