# Request flows

This page traces what the provider sends to the Network app for each Terraform operation, plus short runbooks. Protect and Access reuse the same session under `/proxy/protect/api` and `/proxy/access/api`; see [apps/protect.md](apps/protect.md) and [apps/access.md](apps/access.md).

To watch the real requests, turn on provider debug logging. It logs each request and response with secrets redacted:

```sh
TF_LOG_PROVIDER=debug terraform plan 2>&1 | grep 'wire HTTP'
```

## Configure and login

Terraform calls the provider's `Configure` once per run, before any resource operation. Paths below are relative to the Network surfaces in [architecture.md](architecture.md#1-api-surfaces); `<site>` is the provider's `site`.

1. Resolve settings. Each attribute falls back to its environment variable ([provider configuration](architecture.md#provider-configuration)). A value unknown at configure time, a missing host, or missing credentials fail here, before any network call.
2. Log in with `POST /api/auth/login` as a local UniFi OS admin. The response sets the `TOKEN` JWT cookie and the CSRF token. Re-auth, CSRF rotation, and 429 retries are in [Session lifecycle](architecture.md#session-lifecycle).
3. Probe capabilities. `GET stat/sysinfo` reads the Network version and must succeed. Two optional probes follow: the Protect NVR and `GET /api/userCertificates`. A failed optional probe only clears its flag.
4. Cache the client. Configs with the same host, credentials, site and transport settings share one logged-in client, so a run logs in once ([Login rate limiting](architecture.md#login-rate-limiting)).

## Read and reconcile

`Read` runs on every refresh, plan and import, and turns the controller's object into state:

1. `GET` the object by its `_id` (the state `id`). Surfaces without a GET-by-id are listed and filtered: devices from `stat/device`, and every v2 collection (`static-dns`, AP groups, traffic rules, WAN SLAs). Singletons are read by their fixed key.
2. On not-found, remove the resource from state. The next plan proposes to create it.
3. Otherwise map the object into state, normalized to match config. For example, MACs are lowercased so a case-only difference is not a diff.

`Optional` + `Computed` attributes use `UseStateForUnknown`. An attribute you don't set keeps its prior value when another attribute changes, instead of showing `(known after apply)`. Drift still shows, because each `Read` writes the controller's value into state.

## Create and sparse update

Create is a `POST` to the collection, such as `POST rest/networkconf`. The response carries the new `_id`, which becomes the state `id`.

Update is a sparse `PUT`. The legacy `/rest` PUT merges the body's top-level fields into the stored object, so the provider sends only the fields it owns and the rest keep their stored value. It never sends back a document it read from the controller. When a PUT returns no object, the provider reads it again.

v2 PUTs go to `<path>/<_id>` and carry `_id` in the body too. Some v2 documents replace on write instead of merging; those use the GET-overlay flow below.

## GET-overlay updates

When the controller replaces a field wholesale, a sparse PUT would erase what the provider doesn't model. These writes read the current value, overlay the owned part, and send the result.

### Device port overrides

A device's `port_overrides` is one array that the controller replaces on every write. When `unifi_network_device` manages at least one port, in config or prior state, each update runs:

1. Work out the owned ports: those in config plus those in prior state.
2. `GET stat/device` and take this device's current `port_overrides`. The `rest/device` GET returns too little to use.
3. Merge. Each configured port is overlaid onto its existing entry, keeping keys the provider doesn't model. An owned port no longer in config is dropped, which reverts it to defaults. Ports never managed stay as they are.
4. `PUT rest/device/<_id>` with the merged array and the owned scalars (`name`, LED settings, `mgmt_network_id`).
5. Read the device again from `stat/device`.

With no managed ports, the update is a `PUT` of the scalars only.

Step 5 can briefly return pre-write values; [How writes work](devices-and-ports.md#how-writes-work) covers how the provider handles that, and [devices-and-ports.md](devices-and-ports.md) covers why ports are a map and how the VLAN fields interact.

### Replace-style v2 documents

`unifi_network_mdns` writes into the v2 global network-config document, which replaces on write. The provider reads the document, overlays the mDNS fields, and PUTs the whole document back so its other required fields survive.

## Capability gating

A feature the console lacks shows up as a plan-time error, not an HTTP 4xx during apply. The checks use the capabilities found at configure time, or a read during planning:

- Protect resources fail in `Configure` when Protect did not answer its probe.
- `unifi_console_certificate` fails in `ModifyPlan` when `/api/userCertificates` did not answer its probe.
- `unifi_protect_camera` reads the camera in `ModifyPlan` and rejects settings its model doesn't support.
- `unifi_network_threat_management` fails in `ModifyPlan` when the plan enables it on a console without IPS.

## Import

Import sets the state `id`, then `Read` fills in the rest. The accepted key depends on the resource:

| Key | Resources |
| --- | --- |
| `_id` or `name=<value>` | `unifi_network_network`, `unifi_network_wan`, `unifi_network_wan_sla` |
| MAC or `_id` | `unifi_network_device`, `unifi_network_dhcp_reservation`, `unifi_network_client_ap_lock` |
| MAC or Protect id | `unifi_protect_camera`, `_chime`, `_light`, `_sensor`, `_viewer` |
| Name or id | `unifi_protect_liveview` |
| `<id>/<header_name>` | `unifi_protect_automation` |
| `door_id` | `unifi_access_door`, `unifi_access_door_guard` |
| Fixed id (singleton) | `unifi_network_settings`, `unifi_network_mdns`, `unifi_network_threat_management`, `unifi_protect_nvr_settings` |
| `_id` | Everything else |

The UniFi UI doesn't show `_id`, so some resources also accept a name or a MAC. Every key resolves to the controller's object id. For resources that import only by `_id`, find it with the matching data source or in the `wire HTTP` debug log of a list GET. Each resource page under [resources/](resources/) names its import key.

Use `import {}` blocks with `terraform plan -generate-config-out=generated.tf` rather than writing blocks by hand:

```hcl
import {
  to = unifi_network_network.lan_a
  id = "name=LAN A"
}

import {
  to = unifi_network_device.switch_a
  id = "00:00:5e:00:53:01"
}

import {
  to = unifi_network_dns_record.printer
  id = "0123456789abcdef01234567"
}
```

Importing a device captures every port with a live override, and all of them become managed. Remove a port from the generated config only if you want it reverted.

An import is done when `terraform plan` is empty and a second plan right after it is also empty. Terraform orders the work from references, so a whole site can be imported in one pass.

## Destroy

Destroy never deletes something that is hard to get back, such as an adopted device or a client record.

| Resource | Destroy does |
| --- | --- |
| Most resources | `DELETE` the object. An object already gone counts as success. |
| `unifi_network_device` | Reverts the managed port overrides through the GET-overlay flow. Name, LED and `mgmt_network_id` stay as last applied, and the device stays adopted. With `forget_on_destroy = true` it sends `cmd/devmgr` `delete-device` instead. |
| Protect cameras, lights, sensors, chimes, viewers | Nothing on the controller. With `forget_on_destroy = true` it unadopts the device. |
| `unifi_network_dhcp_reservation` | Sets `use_fixedip = false`. The client record stays. |
| `unifi_network_client_ap_lock` | Clears the AP lock. The client record and any reservation stay. |
| Objects with `attr_no_delete = true` (AP, WLAN and user groups, RADIUS profiles) | Removes it from state only. The controller won't delete its defaults. |
| Access doors and door guards, Protect automations | Removes it from state only. |
| Singletons (settings, mDNS, threat management, NVR settings) | Removes it from state only. Controller values stay. |

Pair `forget_on_destroy = true` with `lifecycle { prevent_destroy = true }` so a stray destroy or replace can't unadopt hardware.

## Runbooks

These use fictional names and documentation ranges (RFC 5737 addresses, RFC 7042 MACs).

### Configure a new device

The controller must see the device as adopted or pending adoption, so power it on and connect it first. Use the MAC on its label.

```hcl
resource "unifi_network_device" "switch_a" {
  mac  = "00:00:5e:00:53:03"
  name = "Switch A"

  port_overrides = {
    "1" = {
      name              = "Uplink"
      native_network_id = unifi_network_network.lan_a.id
    }
    "2" = {
      name              = "Desk"
      native_network_id = unifi_network_network.lan_b.id
      poe_mode          = "auto"
    }
  }
}
```

Apply looks up the MAC in `stat/device` and fails with "Device not found" if the controller can't see it. A pending device is adopted with `cmd/devmgr` `adopt`, read again, then configured through the port-override flow. Apply does not wait for provisioning. Adoption and config writes are not yet verified on real hardware.

To reference a device without managing it, use the `unifi_network_device` data source.

### Add a VLAN

```hcl
resource "unifi_network_network" "lan_b" {
  name         = "LAN B"
  purpose      = "corporate"
  vlan_id      = 10
  subnet       = "192.0.2.1/24"
  dhcp_enabled = true
  dhcp_start   = "192.0.2.50"
  dhcp_stop    = "192.0.2.250"
}
```

Apply sends `POST rest/networkconf`. WLANs, firewall rules, device ports and DHCP reservations refer to the resulting `id`.

### Reserve a DHCP address

```hcl
resource "unifi_network_dhcp_reservation" "nas" {
  mac        = "00:00:5e:00:53:04"
  name       = "nas"
  ip_address = "192.0.2.10"
  network_id = unifi_network_network.lan_b.id
}
```

A reservation lives on the controller's client record for the MAC (`rest/user`). The controller may already hold a record from a past lease, so the provider looks up the MAC first. If a record exists it PUTs `use_fixedip = true` and `fixed_ip`; otherwise it POSTs a new record. The reservation holds whether or not the device is online.

### Add a classic firewall rule

```hcl
resource "unifi_network_firewall_rule" "block_b_to_a" {
  name           = "Block LAN B to LAN A"
  ruleset        = "LAN_IN"
  rule_index     = 20010
  action         = "drop"
  protocol       = "all"
  src_network_id = unifi_network_network.lan_b.id
  dst_network_id = unifi_network_network.lan_a.id
}
```

Apply sends `POST rest/firewallrule`. `rule_index` is required and accepts the controller's full range. It must be unique within the ruleset; apply fails before writing if another rule uses it. Leave gaps (20010, 20020) for later inserts.

### Add a WLAN

```hcl
resource "unifi_network_wlan" "guest" {
  name         = "Example Guest"
  network_id   = unifi_network_network.guest.id
  ap_group_ids = [unifi_network_ap_group.all.id]
  security     = "wpapsk"
  wpa_mode     = "wpa2"
  wpa3_support = true
  pmf_mode     = "required"
  passphrase   = var.guest_psk
  is_guest     = true
  l2_isolation = true
}
```

Apply sends `POST rest/wlanconf`. `passphrase` is sensitive. The SSID broadcasts only once an AP in one of its AP groups is adopted, so end-to-end checks need hardware; see [testing.md](testing.md).
