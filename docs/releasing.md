# Releasing

Releases are automatic. Every push to `main` cuts one, and `.github/workflows/auto-release.yml` reads the git history to decide whether it is a patch or a minor. CI creates tags and GitHub Releases and never pushes a branch; its only merges are Dependabot auto-merges ([Dependabot](#dependabot)).

Merging, releasing, and the [one-time setup](#one-time-setup) are maintainer tasks. Contributors open a PR against the right branch and wait for review ([CONTRIBUTING.md](../CONTRIBUTING.md)).

| Branch | Holds | Releases |
| ------ | ----- | -------- |
| `main` | Released code. | Every push: a patch, or a minor for a `next → main` merge commit. |
| `next` | Features in progress, possibly across many PRs. | Never. |

## What triggers a release

- Every push to `main`, including docs, CI, and dependency merges. There is no path filter or skip label, so a docs-only merge ships a patch.
- A manual run: `gh workflow run auto-release.yml --ref main`.
- A Dependabot auto-merge, which dispatches the workflow itself.

Runs share the `auto-release` concurrency group, so back-to-back merges release one at a time. An in-flight release is never cancelled.

## Which branch to use

Branch off the branch you target:

- A feature (a new resource, data source, or behavior change) branches off `next` and PRs into `next`.
- A bugfix, or a docs, CI, or dependency change, branches off `main` and PRs into `main`.

A feature PR opened against `main` squash-merges and releases on its own as a patch, before the rest of the feature is ready. If `next` is behind `main`, refresh it first ([Keeping `next` current](#keeping-next-current)).

## Shipping a fix

Open a PR into `main`; a maintainer merges it. The push to `main` runs `auto-release.yml`, which:

1. Builds and unit-tests `main` with `GOFLAGS=-mod=readonly`. It never runs `go mod tidy`, so the PR gate enforces tidiness.
2. Computes the next version ([How the bump is decided](#how-the-bump-is-decided)).
3. Creates an annotated tag and pushes it as `github-actions[bot]`.
4. Runs GoReleaser, which builds and publishes the GitHub Release.

Tagging and GoReleaser share one job because a tag pushed with `GITHUB_TOKEN` triggers no other workflow, so a separate tag-triggered release workflow would never fire.

## Shipping a feature

Build the feature as many PRs into `next`. None of them release. When it is ready:

1. Open the release PR from `next` into `main`:
   ```sh
   gh pr create --base main --head next --title "release: <feature>"
   ```
2. The `checks` job in `test.yml` runs on the merged result. It is a required status on `main`, so a broken feature can't land.
3. The maintainer runs `./test-hil.sh read` against a live console and expects a clean plan. It is the best regression check before a release.
4. A maintainer merges with **Create a merge commit**. That makes `next`'s tip the pushed commit's `HEAD^2`, which is what makes the release a minor. A squash merge releases as a patch.

The release then runs as for a fix, with a minor bump (`v0.1.4` → `v0.2.0`).

### Keeping `next` current

Fixes merged into `main` don't reach `next` until someone brings them over; no workflow does it. Open a `main → next` PR, or merge locally:

```sh
git checkout next && git merge main && git push
```

Refresh `next` before starting a feature. A `next → main` release PR shows any conflict up front; resolve it on `next` before merging.

## How the bump is decided

The bump comes from git topology alone, with no PR API, labels, or commit-message parsing:

- `HEAD` is a merge commit and `HEAD^2` is contained in `origin/next`: minor, lower parts reset (`v0.3.2` → `v0.4.0`).
- Anything else (a squash merge, a direct push, or a merge commit whose `HEAD^2` isn't in `next`): patch (`v0.3.2` → `v0.3.3`).

There is no major path yet. Add one before v1.0.

The base is the newest `vX.Y.Z` tag. With no tags, the first release is `v0.0.1` (patch) or `v0.1.0` (minor).

The workflow fails without releasing when:

- `HEAD` already carries a `v*` tag.
- The newest tag isn't `vX.Y.Z`.
- The computed tag already exists.

## What a release contains

GoReleaser (`.goreleaser.yml`) publishes, for `<ver>` without the leading `v`:

| Artifact | Name |
| -------- | ---- |
| Zip per platform: linux, darwin, windows × amd64, arm64 | `terraform-provider-unifi_<ver>_<os>_<arch>.zip` |
| Binary inside each zip | `terraform-provider-unifi_v<ver>` |
| Checksums, covering the zips and the manifest | `terraform-provider-unifi_<ver>_SHA256SUMS` |
| Detached signature of the checksums (when a key is set) | `terraform-provider-unifi_<ver>_SHA256SUMS.sig` |
| Registry manifest (protocol `6.0`) | `terraform-provider-unifi_<ver>_manifest.json` |

The release body is the commit log since the previous tag, one short-hash link per commit, without `deps:`, `ci:`, `chore:`, `docs:`, `test:`, or merge commits. The curated narrative lives in [`CHANGELOG.md`](../CHANGELOG.md).

## Recovering from a failed release

- Fails before the tag push (build or unit tests): nothing was published. Fix forward with a new PR, or re-run once the cause is gone.
- Fails after the tag push (GoReleaser): delete the partial release and the tag, then re-run the failed job:
  ```sh
  gh release delete vX.Y.Z --cleanup-tag --yes   # or: git push origin :refs/tags/vX.Y.Z
  ```
  Re-running the failed job releases the same commit. A fresh `gh workflow run auto-release.yml --ref main` releases whatever `main` points at now.
- Dependabot merged but nothing released (the dispatch failed): run `gh workflow run auto-release.yml --ref main`.
- The Registry missed a release: use Resync on the provider's page at registry.terraform.io.

## Changelog and generated docs

- `CHANGELOG.md` is date-based, not versioned. Add your entry under today's date in the PR that makes the change. Releases never edit it.
- `docs/index.md`, `docs/resources/`, and `docs/data-sources/` are generated from the provider schema by tfplugindocs. Regenerate them with `make docs`; CI fails a PR whose `docs/` is stale.
- `make generate` also regenerates the committed `wire` types. It needs `UNIFI_SPECS` pointing at the version-pinned API spec, which isn't public. Without it, use `make docs`.

## Dependabot

Dependabot runs weekly on Monday (`.github/dependabot.yml`). Each ecosystem gets one grouped PR for minor and patch bumps and one for majors. Go module updates wait out a 7-day cooldown on new versions; security updates skip it.

`dependabot-automerge.yml` squash-merges a Go module PR once the `test` workflow (its `checks` job) passes, then dispatches `auto-release.yml`, because a merge made with `GITHUB_TOKEN` fires no push event. It merges only when all of these hold:

- The PR targets `main` and comes from this repo's `dependabot/go_modules/` branch.
- Every commit is Dependabot's.
- No commit trailer marks a semver-major update.
- The head SHA hasn't moved since CI ran (the merge passes `sha=`).

It never checks out PR code, and it deletes the branch afterward. Major groups and every `github-actions` PR wait for a human, since `GITHUB_TOKEN` can't merge workflow-file edits.

Dependabot never raises the `toolchain` line in `go.mod`, so a new Go standard library advisory turns every Dependabot PR red until someone raises it ([CVE policy](#cve-policy)).

## PR checks

The `checks` job in `.github/workflows/test.yml` runs on every PR and is the required status for merging into `main`. `make check` runs the same set locally with two differences: it always runs unit tests, and it lints without the `acc` and `hil` build tags, so also run `golangci-lint run --build-tags=acc,hil ./...`.

| Check | Enforces | Local |
| ----- | -------- | ----- |
| Secret hygiene | No credentials or private identifiers in tracked files. Also reads the optional, gitignored `scripts/secret-denylist.local`. | `./scripts/check-secrets.sh` |
| gofmt | `gofmt -s` formatting. | `make fmt` |
| `go mod tidy` | `go.mod` and `go.sum` are tidy; the release won't tidy them. | `go mod tidy` |
| Build and vet | Compiles, `go vet` clean. | `go build ./... && go vet ./...` |
| govulncheck | No reachable advisory in dependencies or the standard library, under the `go.mod` toolchain. | `make vulncheck` |
| terraform fmt | `examples/` is canonically formatted. | `make tffmt` |
| Docs current | `docs/` matches the schema. | `make docs` |
| golangci-lint | Lint clean, including `acc` and `hil` test files. | `golangci-lint run --build-tags=acc,hil ./...` |
| Unit tests | `./test-ci.sh` passes. Runs only on PRs into `main`. | `make test` |

All jobs run on GitHub-hosted `ubuntu-latest` runners.

## Supply chain

- Third-party actions are pinned to a full commit SHA, because a re-pointed tag would run in the release job with `contents: write` and the GPG key in memory. The trailing `# vN` comment is informational. To roll a pin by hand, resolve the tag with `gh api repos/<owner>/<repo>/commits/<tag> --jq .sha`.
- GitHub's own `actions/*` use a floating major tag (for example `@v7`), so they get patches without a PR.
- Dependabot's `github-actions` updates bump a SHA and its comment together, and flag new `actions/*` majors.
- Container images in `test/tier2/compose.yaml` are pinned by digest. Dependabot doesn't manage them; refresh by hand with `docker buildx imagetools inspect <image>:<tag> --format '{{.Manifest.Digest}}'`.
- `test.yml` has `contents: read`. Only the release job and Dependabot auto-merge, which never checks out PR code, get `contents: write`.
- The release checkout uses `persist-credentials: false`, so the write token isn't in `.git/config` while dependency code builds and runs tests.
- The release build runs with `-mod=readonly` and no `go mod tidy`, so a stale or tampered module graph fails the build instead of being rewritten.

## CVE policy

Fix every govulncheck finding in the PR that surfaces it, including ones the PR didn't introduce. Fix a standard library advisory by raising the `toolchain` line in `go.mod`, and a dependency advisory by bumping the module. See [CONTRIBUTING.md](../CONTRIBUTING.md#pull-requests). Report vulnerabilities in the provider privately as described in [SECURITY.md](../SECURITY.md).

## One-time setup

### The `next` branch

Every release run fetches `origin/next` to decide the bump, so `next` must exist before the first feature. Create it at `main`'s tip:

```sh
git push origin main:refs/heads/next
```

### Repository settings

- Protect `main` and require the `checks` status.
- Under Actions settings, set fork PR workflows to **Require approval for all external contributors**.
- Automatic deletion of merged branches is optional; Dependabot auto-merge works either way.

### GPG signing key

The workflow signs whenever the `GPG_PRIVATE_KEY` secret is set. Without it, releases ship unsigned: they work for direct download, but the Terraform Registry won't publish them. Use an RSA key without a passphrase, because the Registry rejects Ed25519 and no one is present in CI to type a passphrase.

```sh
gpg --batch --pinentry-mode loopback --passphrase '' --quick-generate-key \
    "terraform-provider-unifi <you@example.com>" rsa4096 sign never

FP=<fingerprint printed above>
gpg --armor --export-secret-keys "$FP" > private.asc
gpg --armor --export             "$FP" > public.asc   # uploaded to the Registry

gh secret set GPG_PRIVATE_KEY < private.asc
```

Back up `private.asc` and the fingerprint in your secret manager, then delete the file: `shred -u private.asc` on Linux, `rm -P private.asc` on macOS. Keep `public.asc` for the Registry.

### Terraform Registry

The provider's address is `registry.terraform.io/dovys/unifi` (`main.go`): namespace `dovys`, type `unifi` from the repo name `terraform-provider-unifi`.

It is published from HCP Terraform: organization `dovys` → Registry → Public namespaces → `dovys`, linked to the GitHub account through the HCP Terraform GitHub App. The namespace's GPG key is the release key. To set it up again:

1. Claim the `dovys` namespace with the GitHub account that owns the repo.
2. Under the namespace's Settings, add `public.asc` as a GPG key.
3. Make sure `GPG_PRIVATE_KEY` is set.
4. Choose **Publish → Provider** and select `terraform-provider-unifi`.

The Registry installs a webhook on the repo and ingests each new non-draft GitHub Release that carries the zips, `_SHA256SUMS`, its `.sig`, and `_manifest.json`. Use Resync to re-ingest a release it missed.
