## What and why

<!-- What changed, then why. Link the issue. -->

## Checklist

See [CONTRIBUTING.md](../CONTRIBUTING.md#pull-requests).

- [ ] Targets `next` for a feature, `main` for a fix, docs, CI, or dependency change
- [ ] `make check` and `golangci-lint run --build-tags=acc,hil ./...` pass
- [ ] Tests cover the change; a fix adds a regression test
- [ ] `CHANGELOG.md` entry under today's date
- [ ] No private data: IPs, MACs, SSIDs, hostnames, credentials
- [ ] Code is LLM-authored from docs/development.md

## Tier 2 acceptance summary

<!-- Paste the summary ./test-extended.sh prints. -->

## Not tested

<!-- Anything you couldn't exercise, such as Tier 3 without a console. -->
