terraform {
  # import {} blocks + -generate-config-out
  required_version = ">= 1.6.0"

  required_providers {
    # Resolved via dev_overrides (run.sh writes dev.tfrc -> the freshly built
    # local binary); the version constraint is intentionally absent because
    # dev_overrides bypasses the registry and the lockfile.
    unifi = {
      source = "dovys/unifi"
    }
  }
}
