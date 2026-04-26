# AUMP Conformance

Cross-language conformance suite for AUMP implementations.

This repository defines the runnable contract that AUMP SDKs, runtimes, and
protocol bridges must pass. It is intentionally separate from `aump-py` and
`aump-js` so one SDK does not become the protocol oracle.

## What It Tests

The current v0.1 suite covers:

- JSON Schema validation for mandates, profiles, and action evaluation payloads.
- Mandate lifecycle checks for active, inactive, and expired mandates.
- Policy evaluation for budget, currency, authority scope, disclosure, and
  escalation decisions.
- MCP metadata and tool annotation bridge checks.
- A2A Agent Card, extension activation, and message metadata bridge checks.
- UCP/AP2 reference boundary checks that prevent full private mandate leakage.
- Evidence event schema checks and runtime evidence semantics for mandate
  matching, retention, and private-field leakage.

The fixtures are pinned to the AUMP spec snapshot in
`fixtures/spec-snapshot.json`.

## Quick Start

```bash
go run ./cmd/aump-conformance validate fixtures
```

Python parity runner:

```bash
uv sync
uv run aump-conformance validate
```

Expected result:

```text
AUMP v0.1 conformance v0.1.0 (spec 0.1.0)
29/29 passed
```

## Report Formats

```bash
go run ./cmd/aump-conformance validate fixtures --format json --output report.json
go run ./cmd/aump-conformance validate fixtures --format junit --output junit.xml
uv run aump-conformance validate fixtures --format json --output report.json
uv run aump-conformance validate fixtures --format junit --output junit.xml
```

## Fixture Layout

```text
fixtures/
  manifest.json
  spec-snapshot.json
  schemas/
  mandates/
  profiles/
  actions/
  bridges/
```

`fixtures/manifest.json` is the canonical case index. SDKs should treat this
manifest as the contract surface, not as example-only data.

## Conformance Levels

The suite maps to the levels described in the AUMP specification:

- Level 1 Parser: schema validation plus inactive and expired mandate rejection.
- Level 2 Policy Evaluator: action decisions for `allowed`,
  `requires_escalation`, and `denied`.
- Level 3 Runtime: bridge metadata and evidence-oriented protocol boundaries.

The first CLI command is:

```bash
aump-conformance validate
```

The native Go runner is the primary conformance executable. The Python runner is
kept as a parity implementation and reference for SDK authors. Future SDK repos
can either shell out to this runner in CI or embed the fixture manifest and
compare their own evaluator output against the expected decisions.

When installed from PyPI, `aump-conformance validate` runs the bundled fixture
corpus. Pass an explicit path to validate a local fixture directory.
