# Site-wide settings use case. `unifi_network_settings` is a singleton whose
# sections are managed à la carte: declare only the blocks you care about, and
# the rest stay untouched (Terraform never reads or writes a section you omit).
#
# Writes shallow-merge the fields you set over the controller's stored section,
# so fields you don't model are preserved. Removing a block later stops managing
# that section — it is left as-is, not reverted.

resource "unifi_network_settings" "site" {
  # Site time zone and country (ISO-3166 numeric code).
  locale = {
    timezone = "America/New_York"
  }

  country = {
    code = 840 # United States
  }

  # Point the site at custom NTP servers instead of the UniFi-provided pool.
  ntp = {
    setting_preference = "manual"
    ntp_server_1       = "0.pool.ntp.org"
    ntp_server_2       = "1.pool.ntp.org"
  }
}
