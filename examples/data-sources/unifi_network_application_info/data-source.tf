# Read the detected controller version and capabilities (from the Configure-time
# probe) — useful for gating configuration on firmware support.
data "unifi_network_application_info" "this" {}

output "controller_version" {
  value = data.unifi_network_application_info.this.version
}

output "integration_writes_supported" {
  value = data.unifi_network_application_info.this.integration_writes
}
