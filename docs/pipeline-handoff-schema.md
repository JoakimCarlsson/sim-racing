# Pipeline HANDOFF schema

This document is the canonical contract for handoffs between the four pipeline agents (planner → coder → smoke-tester → reviewer). It applies identically to all three agent homes (`.github/agents/`, `.cursor/agents/`, `.claude/agents/`). Only the chaining mechanism differs between homes.

A handoff is a fenced block at the end of an agent's output. The orchestrator (or the Cursor/VS Code `handoffs:` frontmatter) parses the block and decides what to do next. **A malformed handoff must not invoke the next LLM stage** — it must escalate to the user.

## Block envelope

Every handoff uses this envelope:

```
---HANDOFF:<TYPE>---
<field>: <value>
<field>: |
  <multi-line value>
---END HANDOFF---
```

Five types: `PLAN`, `IMPLEMENTATION`, `VERIFIED`, `FIX`, `APPROVED`.

Fields use YAML 1.2 scalar / mapping / sequence syntax. The orchestrator parses the body with a YAML parser; the markers themselves are literal text and **not** part of the YAML payload.

## Common required fields

Every handoff block must include:

| Field            | Type    | Notes                                                                 |
|------------------|---------|-----------------------------------------------------------------------|
| `schema_version` | string  | Set to `"1"` for this document. Bump on breaking changes only.        |
| `issue_number`   | integer | GitHub issue number this run is bound to.                             |
| `issue_url`      | string  | Canonical `https://github.com/.../issues/N` URL.                      |
| `next_agent`     | enum    | One of `planner`, `red-team`, `coder`, `smoke-tester`, `reviewer`, `none`. Use `planner` on `HANDOFF:APPROVED` to chain into the milestone-loop's next issue. |

Any handoff missing any of these four fields is **malformed** and must be rejected by the orchestrator.

## `HANDOFF:PLAN` (planner → coder)

Produced by `planner`. The plan is a structured contract, not prose.

```yaml
schema_version: "1"
issue_number: 42
issue_url: https://github.com/JoakimCarlsson/sim-racing/issues/42
issue_title: <title>
milestone: <milestone title or "none">
branch_name: task/42-<slug>
branch_linked: true        # confirmed via `gh issue develop --list 42`

summary: |
  <1-3 sentences: what we are building and why. If LEARNINGS.md has
  applicable entries, name them here explicitly.>

acceptance_criteria:        # mirrors the issue's `- [ ]` checkboxes verbatim
  - id: AC1
    text: "<observable criterion 1>"
  - id: AC2
    text: "<criterion 2>"

files_touched:              # exhaustive — coder may not edit files outside this list without bouncing back
  - path: <relative/path>
    action: create | modify | delete
    notes: <what to do>

interfaces:                 # public APIs, types, routes, env vars introduced or changed
  - kind: route | type | env | cli
    name: <symbol or route>
    signature: <Go signature / HTTP shape / env var name>

test_cases:                 # one or more entries per AC; coder refuses to start if any AC has zero entries
  - ac: AC1
    kind: unit | integration | smoke | manual
    location: <path/to/test or "manual: <step>">
    asserts: <what is being asserted>

non_goals:                  # explicit scope fences
  - <thing we are deliberately not doing>

assumptions:                # claims about the repo or environment the plan rests on; red-team will try to refute each
  - id: A1
    claim: "<assumption text>"
    refutable_by: <grep / file path / command that would refute this if false>

dependencies_and_risks:
  - <risk or dependency>

testing_notes: |
  <how coder/smoke-tester should verify; include E2E curl steps for any HTTP change>

next_agent: coder
```

**Validation rules:**
- Every `id` in `acceptance_criteria[]` must appear at least once as `ac:` in `test_cases[]`. The coder rejects the handoff otherwise.
- `assumptions[]` may be empty. If present, the red-team pass runs against every entry before the coder is invoked.
- `files_touched[]` must be non-empty unless the issue is documentation-only (and the issue must carry the `kind/documentation` label).

## `HANDOFF:IMPLEMENTATION` (coder → smoke-tester)

```yaml
schema_version: "1"
issue_number: <N>
issue_url: <url>
issue_title: <title>
branch_name: <task/N-slug>
pr_url: <PR URL from gh pr create>

plan_reference: |
  <1-2 sentences linking back to HANDOFF:PLAN summary>

changes_made:               # one entry per file actually touched
  - path: <file>
    summary: <what changed>

ac_mapping:                 # which file:line satisfies each AC
  - ac: AC1
    evidence: <path/to/file.go:LINE>

commands_to_verify:
  build: go build ./cmd/api ./cmd/migrate
  test: go test -short ./...
  serve: go run ./cmd/api
  smoke_endpoints:
    - method: GET
      path: /health
      expect: '{"status":"ok"} (200)'

drift_log:                  # short trace of the drift-check status the coder emitted during the run
  - ac: AC1
    file: <path>
    note: <one-line "current AC / current file / why">

environment_notes: |
  <env vars, ports, seed data>

known_gaps:                 # anything intentionally deferred or out of scope
  - <gap>

next_agent: smoke-tester
```

**Validation rules:**
- Every AC id in the originating `HANDOFF:PLAN` must appear in `ac_mapping[]`. If any AC lacks evidence, the coder must instead emit `HANDOFF:FIX` to itself (escalate) — the orchestrator rejects the handoff.
- `pr_url` must look like a GitHub PR URL; the orchestrator validates with `gh pr view <pr_url>`.

## `HANDOFF:VERIFIED` (smoke-tester → reviewer, pass)

```yaml
schema_version: "1"
issue_number: <N>
issue_url: <url>
pr_url: <url>
branch_name: <branch>

build: PASS
unit_tests: PASS — <N> tests

verification:               # one entry per smoke_endpoint or browser route
  - endpoint: GET /health
    status: 200
    content_check: '{"status":"ok"} present'
    result: PASS
  - route: /play            # browser-smoke entry shape
    snapshot: PASS
    screenshot: <path or "n/a">
    console_errors: 0
    result: PASS

blockers: []

implementation_summary: |
  <condensed from HANDOFF:IMPLEMENTATION>

next_agent: reviewer
```

**Validation rules:**
- `verification[]` must be non-empty.
- Every endpoint listed in the prior `commands_to_verify.smoke_endpoints` must appear in `verification[]` with `result: PASS`. Missing endpoint = malformed.
- For diffs touching `web/`, at least one row must be a `route:` (browser-smoke) entry; the smoke-tester must also report `console_errors`.

## `HANDOFF:FIX` (smoke-tester or reviewer → coder)

```yaml
schema_version: "1"
from_agent: smoke-tester | reviewer
issue_number: <N>
issue_url: <url>

failure_summary: |
  <what failed — paste raw command output, or 1-3 sentences for reviewer findings>

failure_signature:          # used by the orchestrator's circuit breaker
  stage: smoke-tester | reviewer
  class: build | unit-test | smoke-endpoint | lint | spec-conformance | review
  symbol: <test name | endpoint path | lint rule id | AC id>

spec_conformance:           # required when from_agent is reviewer
  - ac: AC1
    status: MET | UNMET
    evidence: path/to/file.go:<line> | "<reason nothing covers it>"

required_changes:
  - <specific change>

prior_handoff_plan: |
  <paste original PLAN summary or key acceptance_criteria>

next_agent: coder
```

**Validation rules:**
- `failure_signature` is mandatory. The orchestrator hashes `(stage, class, symbol)` and escalates to the user if the same hash repeats within a single issue run.
- When `from_agent: reviewer`, `spec_conformance[]` must include every AC id from the plan; any `UNMET` row is a hard blocker.

## `HANDOFF:APPROVED` (reviewer → merge → planner | none)

The reviewer merges the PR (squash + delete branch) as part of approval, then either re-invokes the planner on the next open issue in the same milestone (`next_agent: planner`) or stops the loop (`next_agent: none`) when the milestone is empty.

```yaml
schema_version: "1"
issue_number: <N>
issue_url: <url>
issue_title: <title>
pr_url: <url>

review_summary: |
  <2-4 sentences: what was reviewed and why it is acceptable>

spec_conformance:           # every AC must be MET
  - ac: AC1
    status: MET
    evidence: path/to/file.go:<line>

verification_reference: |
  <condensed from HANDOFF:VERIFIED>

non_blocking_notes:
  - <suggestion or nit, if any>

retrospective: |
  <The exact line appended to LEARNINGS.md, or "nothing to record">

merge_result:               # required — outcome of `gh pr merge --squash --delete-branch`
  status: MERGED | SKIPPED | FAILED
  sha: <merge commit SHA, or null when SKIPPED/FAILED>
  notes: <one line — e.g. "merged via squash, branch deleted" or failure reason>

milestone: <milestone title or "none">
next_open_issue: <N or null>   # number of the next open issue in the milestone, or null if empty

next_agent: planner | none     # planner when next_open_issue is set; none when milestone is empty or merge failed
```

**Validation rules:**
- Every AC id from the originating plan must appear with `status: MET`.
- `retrospective` is required (use `"nothing to record"` if nothing is worth keeping).
- `merge_result.status` is required. `MERGED` requires a non-null `sha`. `FAILED` requires `notes` describing the gh error.
- If `merge_result.status != MERGED`, `next_agent` MUST be `none` — do not loop on a failed merge.
- If `next_agent: planner`, `next_open_issue` MUST be a positive integer; the orchestrator (or the reviewer in handoff-frontmatter flows) passes that issue number to the planner.

## Failure-signature hashing

The orchestrator computes:

```
hash = sha1(stage + "|" + class + "|" + symbol)[:12]
```

If the same hash appears twice within an issue run, the orchestrator **does not** re-invoke the coder. It surfaces both failure summaries to the user and stops. This replaces the old "3 retries blindly" policy; the per-stage retry cap (3) is still the hard upper bound.

## Token budget

The orchestrator tracks total tokens spent. In single-issue runs the ceiling applies per issue; in the **milestone-loop** variant the ceiling applies across the entire loop (every issue, every stage, including the merge step). The default is **400 000 tokens**. When the ceiling is exceeded the orchestrator stops and surfaces a summary; the next stage is not invoked, and no new issue is started.

The ceiling is configurable per run via the `BASTION_PIPELINE_BUDGET` environment variable (raw integer of tokens).

## Milestone-loop semantics

When invoked as `/pipeline <milestone-title>` (or when the reviewer's `HANDOFF:APPROVED` is wired through the Cursor/VS Code `handoffs:` frontmatter), the pipeline runs the full chain per issue and uses `HANDOFF:APPROVED.next_agent` to decide what happens after merge:

- `next_agent: planner` — pop the next open issue from the milestone, reset per-issue counters (signature set, attempt counters), reuse the same run log, continue. The planner is invoked with the new `issue_number`.
- `next_agent: none` — milestone is empty (or the merge failed). Print summary, stop.

Per-issue `signatures_seen` resets between issues. The token budget does **not** reset. Failures, malformed handoffs, red-team REFUTED, repeat signatures, retry-cap hits, and merge failures all stop the whole loop — they never silently advance to the next issue.

## Validation contract

The orchestrator (and the Cursor/VS Code handoff trigger) MUST:

1. Reject blocks where the envelope markers are missing or unbalanced.
2. Reject blocks where the body is not valid YAML.
3. Reject blocks where any of the common required fields are missing.
4. Reject blocks where any type-specific required field is missing (per the rules above).
5. Compute the failure signature for every `HANDOFF:FIX` and break the loop on repeat.
6. Validate `pr_url` with `gh pr view` before invoking the next stage on `HANDOFF:IMPLEMENTATION`.

A malformed handoff is a **stop-and-surface** event, not a retry event.
