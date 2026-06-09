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

**Branch off the branch you target — this is a hard rule.** A **feature** (any new
resource, data source, or behavior change — anything worth a minor/major bump)
**branches off `next`** and PRs into `next`. A **bugfix**, or a **docs / CI / dep-only**
change, **branches off `main`** and PRs into `main`. Never branch a feature off `main`:
its base would be `main`'s tip, which muddies the eventual `next → main` release diff
and risks shipping a half-staged feature as a patch. If `next` is behind `main` when you
start a feature, refresh it first ([Keeping `next` current](#keeping-next-current)) —
don't reach for `main` because it's newer.

### Patch lane — zero effort

Branch off **`main`** and open a fix PR against **`main`**. On merge, `.github/workflows/auto-release.yml`:

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

Develop the feature as as many PRs as you like, each **branched off `next`** and
targeting **`next`**. None of them release. When the feature is ready, ship it by merging `next` into `main`
**through a PR** — you click Merge, CI does the rest:

1. Open the release PR: base **`main`**, compare **`next`**:
   ```sh
   gh pr create --base main --head next --title "release: <feature>"
   ```
   (or use GitHub's *Compare & pull request*). It's a **minor** — there's no major
   path yet (add one when v1.0 nears).
2. `test.yml` runs on the PR and tests the **merged result** — so a feature that
   breaks the build literally cannot land on `main`. Merge only when green, using
   **"Create a merge commit"** — this is **load-bearing**, not just for tidy release
   notes: the bump is read from git topology, and the merge commit is what makes the
   merged-in tip of `next` the pushed commit's `HEAD^2`. A squash erases that, and the
   release would read as a patch.
3. The merge pushes `main`, which triggers `auto-release.yml`. It reads the bump from
   **git alone**: the pushed commit is a merge whose `HEAD^2` is contained in `next`
   → **minor**, lower parts reset to zero (`v0.1.x` → **`v0.2.0`**). It then tags and
   runs GoReleaser. **No `git merge`, no branch push** — only the tag is pushed by CI.

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

- **Every push to `main` cuts a release** — docs, CI, and dependency merges
  included. There's no skip path and no path filter (simple = solid); a docs-only
  merge ships a patch with refreshed artifacts. So don't push trivia straight to
  `main` you don't want tagged.
- The bump is decided **purely from git topology** — no PR API (its commit→PR
  association lags and once mis-bumped a minor to a patch), no labels, no commit
  strings. A `next → main` **merge commit** (its `HEAD^2` is contained in `next`) →
  **minor**; everything else (a squash-merged fix/docs/dep, a direct push, or a fix
  merged as a merge commit whose `HEAD^2` isn't in `next`) → **patch**. No automatic
  minor without a real `next → main` merge commit, and no major path yet.

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

- **3rd-party actions pinned to a full commit SHA**, never a mutable tag — a
  re-pointed tag is the classic Actions compromise (it would run in the release
  job with `contents: write` and, once configured, the GPG key in memory).
  **GitHub's own `actions/*`** (checkout, setup-go, cache) ride a floating major
  tag (`@v6`) — we trust the publisher and it keeps them auto-patched. The trailing
  `# v7` comment on a pinned action is informational; **Dependabot**
  (`github-actions`) bumps the SHA + comment together and floats the `actions/*`
  majors. To roll a pinned action by hand, resolve tag → SHA with
  `gh api repos/<owner>/<repo>/commits/<tag> --jq .sha`.
- **Tier-2 stack images pinned by digest** (`test/tier2/compose.yaml`: Mongo,
  controller, nginx bridge) — refresh deliberately with
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
