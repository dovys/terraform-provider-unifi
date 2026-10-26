# Security policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub's
[private vulnerability reporting](https://github.com/dovys/terraform-provider-unifi/security/advisories/new).
Don't open a public issue, pull request, or discussion for a security problem.

Include:

- the provider version, and the Terraform/OpenTofu and UniFi OS/app versions;
- what an attacker can do, and the conditions needed (network position, account,
  configuration);
- steps or a minimal config to reproduce it — with your own credentials,
  addresses, and MACs scrubbed;
- whether secrets end up somewhere they shouldn't (state, plan output, logs).

You'll get an acknowledgement, and the fix ships in a patch release with a
published advisory.

## Supported versions

Only the latest release receives security fixes.
