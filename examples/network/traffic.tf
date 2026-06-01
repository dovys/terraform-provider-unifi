# Traffic management use case: shape and steer traffic by destination.
#
# Two common shapes — block ad/tracker domains for everyone, throttle a streaming
# category, and route a streaming service out a VPN.

# Block a set of ad/tracker domains for all clients.
resource "unifi_network_traffic_rule" "block_ads" {
  action          = "BLOCK"
  matching_target = "DOMAIN"
  description     = "Block ads/trackers"
  domains         = ["ads.example.com", "tracker.example.net"]

  target_devices = [
    { type = "ALL_CLIENTS" },
  ]
}

# Rate-limit a streaming app category for the kids' devices.
resource "unifi_network_traffic_rule" "throttle_streaming" {
  action          = "SPEED_LIMIT"
  matching_target = "APP_CATEGORY"
  description     = "Throttle streaming on kids' devices"
  # App-category ids are controller-defined; look them up via the UI/API.
  app_category_ids = [4]

  target_devices = [
    { type = "CLIENT", client_mac = "aa:bb:cc:00:11:22" },
  ]

  bandwidth_limit = {
    enabled             = true
    download_limit_kbps = 5000
    upload_limit_kbps   = 2000
  }
}

# Route Netflix out a VPN network instead of the default WAN, dropping the
# traffic if the VPN is down (kill switch) rather than leaking over the WAN.
resource "unifi_network_traffic_route" "netflix_via_vpn" {
  matching_target     = "DOMAIN"
  description         = "Netflix via VPN"
  domains             = ["netflix.com", "nflxvideo.net"]
  network_id          = unifi_network_network.iot.id # the VPN/interface network to egress through
  kill_switch_enabled = true

  target_devices = [
    { type = "ALL_CLIENTS" },
  ]
}
