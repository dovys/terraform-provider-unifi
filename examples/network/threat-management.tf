# Threat Management (IDS/IPS) use case. `unifi_network_threat_management` is the
# site `ips` settings singleton: `mode` runs the engine in detection (`ids`) or
# prevention (`ips`/`ipsInline`), and `enabled_categories` selects which threat
# signature categories are active.
#
# Writes shallow-merge the fields you set over the controller's stored section,
# so per-network honeypot entries and any field you don't model are preserved.
# Destroy removes the resource from state without disabling Threat Management.

resource "unifi_network_threat_management" "main" {
  # Prevention: block matching traffic. Use "ids" to detect/log only, or
  # "disabled" to turn the engine off.
  mode = "ips"

  # Block BitTorrent / peer-to-peer traffic.
  restrict_torrents = true

  # Enable a focused set of high-signal signature categories.
  enabled_categories = [
    "emerging-malware",
    "emerging-exploit",
    "botcc",
    "phishing",
    "tor",
  ]
}
