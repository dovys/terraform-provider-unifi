# Operator flows

End-to-end flows that define the expected UX; schemas in [apps/network.md](apps/network.md) must make
them this clean.

## A — Join & configure a new device

1. Rack/power it so it reaches the controller → appears **pending** within ~1 min.
2. Read its MAC from the label.
3. Declare one device resource:
   ```hcl
   resource "unifi_network_device" "office_switch" {
     mac               = "00:00:5e:00:53:03"   # example MAC (RFC 7042) — use the one on the label
     wait_for_adoption = "5m"
     name              = "Office Switch"
     port_overrides = {
       "1" = { name = "Uplink",   native_network_id = unifi_network_network.mgmt.id }
       "2" = { name = "Desk PoE", native_network_id = unifi_network_network.trusted.id, poe_mode = "auto" }
     }
   }
   ```
4. `apply` → adopts, waits for provisioned, then sparse-PUTs name + the merged `port_overrides`.

Variations: `wait_for_adoption = "10m"` lets you apply before powering on; default `0` errors if
undiscovered (so success means real hardware). Destroying the resource reverts the config and leaves
the device adopted; add `forget_on_destroy = true` (guard with `prevent_destroy`) only if you want
destroy to forget it. To reference a device without managing it, use `data.unifi_network_device`.

## B — Add a VLAN
```hcl
resource "unifi_network_network" "cameras" {
  name = "cameras"  purpose = "corporate"  vlan = 150
  subnet = "192.0.2.1/24"  dhcp_enabled = true
  dhcp_start = "192.0.2.50"  dhcp_stop = "192.0.2.250"
}
```
`POST /rest/networkconf`; the `id` is then referenceable by WLANs, firewall rules, ports, reservations.

## C — Static DHCP reservation
```hcl
resource "unifi_network_dhcp_reservation" "nas" {
  mac = "00:00:5e:00:53:04"  name = "nas"
  fixed_ip = "192.0.2.10"    network_id = unifi_network_network.servers.id
}
```
**Find-or-create by MAC**: the controller auto-creates a `user` per seen MAC, so the provider GETs by
MAC and PUTs `use_fixedip=true` (POST only if absent). Destroy clears the reservation, not the user.
Persists whether or not the device is online.

## D — Classic firewall rule
```hcl
resource "unifi_network_firewall_rule" "block_iot_to_servers" {
  name = "Block IoT to servers"  ruleset = "LAN_IN"  rule_index = 20010
  action = "drop"  protocol = "all"
  src_network_id = unifi_network_network.iot.id  dst_network_id = unifi_network_network.servers.id
}
```
`POST /rest/firewallrule`. Explicit `rule_index` (full range); a plan-time guard rejects duplicates
within a ruleset — leave gaps (20010, 20020) for insertion. Migrating to zones means switching to
`unifi_network_firewall_zone` + `unifi_network_firewall_policy` (the provider refuses to mix the two).

## E — WLAN
```hcl
resource "unifi_network_wlan" "guest" {
  name = "Guest WiFi"  network_id = unifi_network_network.guest.id
  ap_group_ids = [unifi_network_ap_group.all.id]
  security = "wpapsk"  wpa_mode = "wpa2"  wpa3_support = true  pmf_mode = "required"
  passphrase = var.guest_psk   # Sensitive
  is_guest = true  l2_isolation = true
}
```
`POST /rest/wlanconf`. ⚠ A WLAN only radiates with a real AP bound, so end-to-end checks need
hardware (see [testing.md](testing.md)).

## F — Import an existing network

1. Discover IDs via data sources (or the controller's `_id`s).
2. Write the resource block to match current config.
3. `terraform import`:
   ```
   terraform import unifi_network_network.mgmt   0123456789abcdef01234567   # state id = _id (example)
   terraform import unifi_network_device.gateway 00:00:5e:00:53:01          # MAC-keyed (example MAC)
   terraform import unifi_network_dns_record.host1 89abcdef0123456789abcdef
   ```
   MAC-keyed resources store the MAC as state `id` and resolve MAC→`_id` via custom `ImportState`;
   everything else stores the controller `_id`. Each resource's docs state its import key.
4. `plan` until empty — and a **second** consecutive plan is also empty (the round-trip bar).

Dependency order for importing a whole site: networks (LANs → WANs) → user_groups + ap_groups →
wlans → devices → firewall_groups → firewall_rules → dhcp_reservations → dns_record → radius_profile
+ settings. The site is "fully managed" when `plan` over the whole config is empty.
