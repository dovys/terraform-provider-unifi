# Releasing

Releases are **zero-effort**: merging a substantive PR to `main` cuts the next
patch version automatically. There is no manual tag step and nothing is ever
committed back to `main` by the release process.

## What happens on merge to `main`

`.github/workflows/auto-release.yml` runs on every push to `main` and:

1. Builds and unit-tests `main` (insurance — a broken `main` never releases).
2. Computes the next version: the newest `vX.Y.Z` tag with **patch +1**
   (`v0.1.0` → `v0.1.1` → …).
3. Creates an annotated tag and pushes it.
4. Runs GoReleaser, which builds binaries for every OS/arch
   (darwin/freebsd/linux/windows × amd64/386/arm/arm64, minus darwin/386),
   zips them, writes `..._SHA256SUMS`, GPG-signs the checksum (if a key is
   configured — see below), bundles `terraform-registry-manifest.json`, and
   creates the **GitHub Release** with the binaries attached.

Tagging and GoReleaser run in the **same job** on purpose: a tag pushed with the
default `GITHUB_TOKEN` does not trigger other workflows, so a separate
tag-triggered release workflow would silently never fire.

### Release notes

The GitHub Release body is auto-derived by GoReleaser from the commit log since
the previous tag, with `deps:`/`ci:`/`chore:`/`docs:`/`test:`/merge commits
filtered out. The curated human narrative lives in [`../CHANGELOG.md`](../CHANGELOG.md).

## What triggers — and what doesn't

Releases fire on substantive merges. **Non-functional merges are skipped** via
`paths-ignore` — a push whose changed files are *entirely* within `**/*.md`,
`docs/**`, `.github/**`, or `go.mod`/`go.sum` does not release (dependabot
bumps, CI tweaks, and docs-only changes). A change that also touches Go source
still releases.

Manual override: put **`[skip release]`** in the merge commit message to skip a
release you don't want.

## Keeping the changelog and docs current

- **CHANGELOG.md** is date-based and decoupled from versions — add your entry
  under today's date **in the PR that makes the change**. The release process
  never touches it.
- **Docs** (`docs/resources/*`, `docs/data-sources/*`) are regenerated from the
  provider schema. A CI gate in `test.yml` fails any PR whose `docs/` is stale,
  so `main` always carries current docs at merge. Regenerate locally with:
  ```
  make docs        # tfplugindocs only — NOT `go generate ./...`
  ```
  > `make generate` (`go generate ./...`) additionally runs the `wire` codegen,
  > which needs the sibling `../unifi-specs` repo. CI runs **only** `make docs`.

## One-time setup

### GPG signing (registry-grade artifacts)

Until a key is configured, releases ship **unsigned** but fully functional. To
enable signing, generate a **passphraseless** key (a passphrase is dead weight
in a fully automated flow — there's no human to type it) and store the private
key in the repo secrets, with a backup in 1Password:

```sh
# Passphraseless key (RSA 4096) — note the fingerprint it prints.
gpg --batch --pinentry-mode loopback --passphrase '' --quick-generate-key \
    "terraform-provider-unifi <1223251+dovys@users.noreply.github.com>" rsa4096 sign never

FP=<fingerprint>
gpg --armor --export-secret-keys "$FP" > private.asc   # for the repo secret + 1Password
gpg --armor --export             "$FP" > public.asc    # for the Registry, later

gh secret set GPG_PRIVATE_KEY < private.asc            # the only secret needed
op item create --category 'Secure Note' --title 'terraform-provider-unifi GPG signing key' \
    "private key[file]=private.asc" "public key[file]=public.asc" "fingerprint=$FP"

shred -u private.asc                                   # remove the on-disk copy
```

The workflow signs whenever `GPG_PRIVATE_KEY` is present and skips signing
otherwise — **no `PASSPHRASE` secret** is used.

### Terraform Registry (GATED — requires the repo to be public)

The public Terraform Registry only publishes from **public** GitHub repos; there
is no private-repo path. This repo is private, so publishing is **not** wired up
and is gated on a separate decision (and the permission) to make it public. The
release artifacts are already Registry-valid, so activation is a one-time manual
step when cleared:

1. Make the repo public.
2. Sign in to `registry.terraform.io` with GitHub (`dovys`).
3. Publish → Provider → select `terraform-provider-unifi` (namespace `dovys`,
   type `unifi` — matches `registry.terraform.io/dovys/unifi` in `main.go`).
4. Upload the GPG **public** key (`public.asc`) to the namespace's signing keys.
5. Ensure `GPG_PRIVATE_KEY` is set so releases are signed.

Thereafter each new signed GitHub Release auto-publishes — still zero per-release
effort.
