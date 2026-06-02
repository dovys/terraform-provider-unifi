# Contributing

This provider is built clean-room and entirely by an LLM, so the best way to help
is probably not what you'd expect. Please read this before you start.

## 1. Open a detailed issue, not a pull request

The most valuable contribution here is a **precise issue** — that's exactly what
the project's process needs to produce a correct fix or feature. A good issue
lets us reproduce and model the behavior without access to your network:

- **UniFi versions** (Network / Protect / UniFi OS), **provider version**, and
  **Terraform version**.
- A **minimal config** that reproduces the problem.
- **What happened vs. what you expected** — full Terraform output and errors.
- If you can get them, the **request the provider sent and the controller's
  response** (redact secrets and public IPs).

For a new feature, describe the UniFi feature, where it lives in the UI, and the
fields you care about.

> Security issue? Don't open a public issue — report it privately to the
> maintainer.

## 2. All code is LLM-authored

Every line — schema, client, tests, docs — is authored by an LLM, by design, and
contributed code is no exception. If you propose a change as code, it must be
produced by an LLM following the project charter:
[`docs/development.md`](docs/development.md) (invariants, resource recipe,
Definition of Done) and [`docs/architecture.md`](docs/architecture.md).

Two hard rules: it's **clean-room** (the API model is derived from UniFi's own
software, never copied from existing code), and wire types are **generated from a
version-anchored spec** — never hand-edited.

## 3. Every bug fix ships with a test

A bug isn't fixed until a test covers it. Because the client is regenerated when
the provider moves to a newer UniFi spec, a one-off fix can be silently undone —
so each fix needs a regression test that **fails** before and **passes** after,
keeping the bug from resurfacing.

See [`docs/testing.md`](docs/testing.md) for the testing tiers and conventions.
