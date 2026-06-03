# Host + credentials come from the environment (run.sh exports UNIFI_HOST,
# UNIFI_USERNAME, UNIFI_PASSWORD, UNIFI_ALLOW_INSECURE). Nothing sensitive ever
# lands in HCL or state config here.
provider "unifi" {}
