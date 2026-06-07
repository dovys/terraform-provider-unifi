variable "unifi_host" {
  type        = string
  description = "UniFi controller URL, e.g. https://192.168.30.1 (the gateway). No /api suffix."
}

variable "unifi_site" {
  type        = string
  description = "UniFi site name."
  default     = "default"
}

# Wi-Fi passphrases are write-only — set via TF_VAR_* only to change a key.
variable "wifi_main_passphrase" {
  type      = string
  sensitive = true
  default   = ""
}

variable "wifi_guest_passphrase" {
  type      = string
  sensitive = true
  default   = ""
}

variable "wifi_iot_passphrase" {
  type      = string
  sensitive = true
  default   = ""
}

# Token the console uses for ACME DNS-01 (console.tf). Write-only; only needed at
# (re)issuance.
variable "dns_api_token" {
  type      = string
  sensitive = true
  default   = ""
}
