# Releasing

Releases follow a **two-branch model** so small fixes ship themselves while
multi-PR features release once, on purpose. **CI only tags and builds the
release — it never merges or pushes a branch.** Every merge is GitHub's merge
button; the bump is derived from which branch the PR came from.

| Branch | Purpose | Releases? |
| ------ | ------- | --------- |
| `main` | Only released code. | **Yes** — every substantive push releases; the bump comes from the merged PR (fix → patch, `next` → minor/major). |
| `next` | Staging for features (possibly many PRs). | **No** — pushes here never release. |

## The two lanes

### Patch lane — zero effort

Open a fix PR against **`main`**. On merge, `.github/workflows/auto-release.yml`:

1. Builds and unit-tests `main` with `GOFLAGS=-mod=readonly` (a broken `main`
   never releases; the release never runs `go mod tidy`, so module tidiness is
   enforced earlier, by the PR gate).
2. Computes the next version: newest `vX.Y.Z` tag with **patch +1** (`v0.1.2` → `v0.1.3`).
3. Creates an annotated tag and pushes it.
4. Runs GoReleaser → binaries for the supported platforms (linux/macOS/windows ×
   amd64/arm64), `..._SHA256SUMS`, the `terraform-registry-manifest.json`, an
   (optional) GPG signature, and the **GitHub Release**.

Tagging and GoReleaser run in the **same job** on purpose: a tag pushed with the
default `GITHUB_TOKEN` does not trigger other workflows, so a separate
tag-triggered release workflow would silently never fire.

### Feature lane — merge a `next → main` PR

Develop the feature as as many PRs as you like, all targeting **`next`**. None of
them release. When the feature is ready, ship it by merging `next` into `main`
**through a PR** — you click Merge, CI does the rest:

1. Open the release PR: base **`main`**, compare **`next`**:
   ```sh
   gh pr create --base main --head next --title "release: <feature>"
   ```
   (or use GitHub's *Compare & pull request*). For a **breaking** release, add the
   **`release:major`** label; otherwise it's a minor. (The `release:major`, cosmetic
   `release:minor`, and `release:skip` labels are pre-created in the repo —
   `release:major` makes the bump major, `release:skip` skips the release entirely,
   and minor is the default for any `next → main` merge.)
2. `test.yml` runs on the PR and tests the **merged result** — so a feature that
   breaks the build literally cannot land on `main`. Merge only when green, using
   **"Create a merge commit"** (keeps the per-feature commits in the release notes).
3. The merge pushes `main`, which triggers `auto-release.yml`. It looks up the PR
   behind the merge: head branch `next` → **minor** (or **major** with the label),
   with the lower parts reset to zero — `v0.1.x` → **`v0.2.0`**, major → **`v1.0.0`**.
   It then tags and runs GoReleaser. **No `git merge`, no branch push** — only the
   tag is pushed by CI.

### Keeping `next` current

Patches that land on `main` aren't on `next` until you bring them over. There's no
automatic CI sync (CI never writes branches). Refresh `next` whenever you want the
latest fixes — e.g. when starting the next feature:

```sh
git checkout next && git merge main && git push      # or open a main → next PR
```

A `next → main` release PR shows any real conflict up front; resolve it on `next`
before merging.

## What triggers — and what doesn't

- The **patch lane** fires only on substantive merges to `main`. **Non-functional
  merges are skipped** via `paths-ignore` — a push whose changed files are
  *entirely* within `**/*.md`, `docs/**`, `.github/**`, or `go.mod`/`go.sum` does
  not release (dependabot bumps, CI tweaks, docs-only changes). A change that also
  touches Go source still releases.
- Manual override: add the **`release:skip`** label to a PR to land its change on
  `main` without cutting a release (e.g. a build-config tweak not worth a version).
  Skip is a PR label, same axis as the bump — no magic commit strings to remember.
- The bump is decided purely by the merged PR's source branch (`next` → minor/major,
  anything else → patch) — there is no automatic minor without a `next → main` merge.

## Release notes

The GitHub Release body is auto-derived by GoReleaser from the commit log since
the previous tag, each line a clickable short-hash commit link, with
`deps:`/`ci:`/`chore:`/`docs:`/`test:`/merge commits filtered out. The curated
human narrative lives in [`../CHANGELOG.md`](../CHANGELOG.md).

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

## PR checks (`test.yml`)

Every PR must pass, so `main` is always releasable without per-release fixups.
Reproduce the whole set locally with **`make check`**:

| Check | Enforces |
| ----- | -------- |
| build & `go vet` | compiles and vets clean |
| `gofmt -s` | Go formatting (`make fmt`) |
| `go mod tidy` clean | `go.mod`/`go.sum` already tidy — the release won't tidy for you |
| **govulncheck** | no known vuln reachable in our call graph — deps **and** the Go standard library (scanned under the patched toolchain, pinned via `GOTOOLCHAIN` in the job until setup-go's `stable` catches up) |
| **terraform fmt** | `examples/*.tf` canonically formatted (`make fmt`) |
| docs current | `docs/` matches the schema (`make docs`) |
| golangci-lint | lint clean |
| unit tests | `go test ./...` green |

## Supply-chain hardening

- **Actions pinned to a full commit SHA**, never a mutable tag (`@v6`) — a
  re-pointed tag is the classic Actions compromise (it would run in the release
  job with `contents: write` and, once configured, the GPG key in memory). The
  trailing `# v6` comment is informational; **Dependabot** (`github-actions`)
  bumps the SHA and the comment together. To roll an action by hand, resolve the
  tag → SHA with `gh api repos/<owner>/<repo>/commits/<tag> --jq .sha`.
- **Service-container images pinned by digest** (`acceptance.yml` Mongo +
  controller) — refresh deliberately with
  `docker buildx imagetools inspect <image>:<tag> --format '{{.Manifest.Digest}}'`.
- **Release build is `-mod=readonly` with no `go mod tidy`** — a tampered or
  stale module graph fails the build instead of being silently rewritten while a
  release is being signed and published.

## One-time setup

### The `next` branch

The feature lane needs a `next` branch to exist:

```sh
git push origin main:refs/heads/next     # create next at the current main tip
```

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
