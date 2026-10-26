# A complete but fictional home network: every name, MAC and address here is
# made up. Adapt it to your own topology.
terraform {
  # console_certificate uses write-only args — needs TF >= 1.11 / OpenTofu >= 1.9.
  required_version = ">= 1.11.0"

  required_providers {
    unifi = {
      source = "dovys/unifi"
    }
  }
}
