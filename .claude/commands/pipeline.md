---
description: Run the full delivery pipeline (planner → red-team → coder → smoke-tester → reviewer) against a GitHub issue. Bridges Claude Code's missing auto-handoff button by invoking each subagent in sequence via the Agent tool, validating typed HANDOFF blocks, breaking on repeat failures, and logging every stage to .pipeline-runs/.
argument-hint: <issue-number>
---

# /pipeline — hardened delivery loop

You are the **pipeline orchestrator**. Invoke each agent in turn via the `Agent` tool, parse the `HANDOFF:*` block in their final output against the schema in `docs/pipeline-handoff-schema.md`, and route to the next agent based on the verdict.

Issue number for this run: **$ARGUMENTS**

## How chaining works in Claude Code

`.cursor/agents/` and `.github/agents/` use frontmatter `handoffs:` to fire the next agent automatically. Claude Code does not have that mechanism — subagents return a single summary and stop. This slash command bridges the gap.

Every agent in this pipeline ends with one of these typed blocks (full field list in `docs/pipeline-handoff-schema.md`):

- `HANDOFF:PLAN` (planner → red-team → coder)
- `HANDOFF:IMPLEMENTATION` (coder → smoke-tester)
- `HANDOFF:VERIFIED` (smoke-tester → reviewer)
- `HANDOFF:FIX` (smoke-tester or reviewer → coder)
- `HANDOFF:APPROVED` (reviewer → user)

Treat the block as canonical — pass it verbatim to the next agent in the prompt.

## Run setup (do this once at the very start)

1. Pick a `run_id`: `YYYYMMDD-HHMMSS-<6-hex>` (UTC, e.g. `20260523-140812-a3f9b1`). Use `Bash` to compute it.
2. Create the log directory: `.pipeline-runs/$ARGUMENTS/`.
3. Open an empty `run.jsonl` at `.pipeline-runs/$ARGUMENTS/<run_id>.jsonl`.
4. Initialise counters in memory: `tokens_total = 0`, per-stage `attempt = 0`, `signatures_seen = {}`.
5. Resolve the token budget: read `$env:BASTION_PIPELINE_BUDGET` (PowerShell) or `$BASTION_PIPELINE_BUDGET` (bash); default to **400000** if unset.

## Validation contract (apply at every stage boundary)

Before invoking the next stage, run these checks against the block returned by the prior stage:

1. **Envelope:** the block is fenced by `---HANDOFF:<TYPE>---` and `---END HANDOFF---` on their own lines.
2. **YAML:** the body parses as YAML (use a tiny Bash one-liner with `python -c` or `yq` if needed).
3. **Common fields:** `schema_version`, `issue_number`, `issue_url`, `next_agent` are all present.
4. **Type-specific fields:** every required field for the block type per `docs/pipeline-handoff-schema.md` is present and non-empty.
5. **Cross-references:** for `HANDOFF:IMPLEMENTATION` and beyond, every AC id from the originating `HANDOFF:PLAN` must appear in `ac_mapping[]` / `spec_conformance[]`.

**Malformed = stop and surface.** Never invoke the next LLM stage on a malformed handoff. Log the row with `verdict: "ERROR"` and `notes: "<which rule failed>"`.

## Failure-signature circuit breaker

Every `HANDOFF:FIX` carries a `failure_signature: { stage, class, symbol }`. Compute the hash:

```
hash = sha1(stage + "|" + class + "|" + symbol)[:12]
```

Maintain a `signatures_seen` set across the run. **If the hash is already in the set, stop and surface.** Paste both failure summaries (the prior one and the current one) so the user can decide. Do not re-invoke the coder on a repeat signature — that is the runaway-loop guard.

The per-stage `attempt` counter is still hard-capped at 3 as a backstop, but the signature check fires first on any repeat.

## Token-budget ceiling

After each stage returns, add `tokens_in + tokens_out` to `tokens_total`. If `tokens_total > budget`, stop and surface. Log the row with `verdict: "ERROR"` and `notes: "budget exceeded: <tokens_total>/<budget>"`. Do not invoke the next stage.

## Logging (append one row per stage)

After every Agent invocation (success or failure), append one JSON line to the run log. Schema and fields are in `docs/pipeline-observability.md`. Use PowerShell `Add-Content -Path <log> -Value "<json>" -Encoding utf8` on Windows, or `>>` on POSIX. Keep the JSON on a single line.

Tokens: read from the Agent tool's reported usage. If unavailable, write `0` and add `notes: "token usage unavailable"`.

## Sequence

### Stage 1 — planner

Invoke `Agent` with `subagent_type: planner` and the prompt:

> Run the planner workflow for issue #$ARGUMENTS. Read AGENTS.md, .claude/agents/_bastion-conventions.md, docs/backend-architecture.md, docs/pipeline-handoff-schema.md, and LEARNINGS.md before drafting. Create the linked task branch via `gh issue develop`. Produce a **structured** plan per the schema (acceptance_criteria with ids, files_touched, interfaces, test_cases, non_goals, assumptions). End your response with the full `HANDOFF:PLAN` block.

Wait for return. Validate the block. Log the stage row.

### Stage 2 — red-team

Invoke `Agent` with `subagent_type: red-team` and the prompt:

> You are the **red-team** for this pipeline run. Your only job: walk every entry in `assumptions[]` from the plan below and attempt to refute it by reading the repo. For each assumption, run the `refutable_by` command (or grep/read the cited file) and report:
>
> - `id: <A1>` — UPHELD (no contradiction found) or REFUTED (with the file:line or command output that contradicts it)
>
> Do not write code. Do not invoke other agents. End with a single line: `RED-TEAM:UPHELD` or `RED-TEAM:REFUTED` plus a one-paragraph summary.
>
> <paste full HANDOFF:PLAN block>

If the result is `RED-TEAM:REFUTED`, stop and surface to the user — the plan rests on a wrong premise and must go back to the ambiguity gate (issue refinement), not to the coder. Log the row.

If `RED-TEAM:UPHELD`, proceed to Stage 3.

### Stage 3 — coder

Invoke `Agent` with `subagent_type: coder` and the prompt:

> Implement the plan below. **Refuse to start** if any `acceptance_criteria[].id` is missing from `test_cases[]` — emit a short `HANDOFF:FIX` back to planner instead. Tests-first for any change under `internal/<subsystem>/` that is pure domain logic. Run the drift-check after every batch of edits: state current AC id, current file, and why this edit advances that AC. End with the full `HANDOFF:IMPLEMENTATION` block including the PR URL, `ac_mapping[]`, and `drift_log[]`.
>
> <paste full HANDOFF:PLAN block>

Wait for return. Validate. Log.

### Stage 4 — smoke-tester

Invoke `Agent` with `subagent_type: smoke-tester` and the prompt:

> Run smoke tests against the implementation below. Observer/recorder only — no fixes. End with `HANDOFF:VERIFIED` on pass or `HANDOFF:FIX` on fail. Every `HANDOFF:FIX` must include `failure_signature: { stage, class, symbol }`.
>
> <paste full HANDOFF:IMPLEMENTATION block>

Wait for return. Validate. Log.

- `HANDOFF:VERIFIED` → proceed to Stage 5.
- `HANDOFF:FIX` → run circuit-breaker check. New signature → loop back to Stage 3 (coder) with the FIX block, increment smoke-fix counter. If signature is a repeat → stop and surface. If counter > 3 → stop and surface.

### Stage 5 — reviewer

Invoke `Agent` with `subagent_type: reviewer` and the prompt:

> Review PR <pr_url>. Wait for CI green before reading the diff. Run the spec-conformance pass (cite `file:line` per AC or mark UNMET). Any `HANDOFF:FIX` must include `failure_signature: { stage: reviewer, class, symbol }`. On CLEAN, append the retrospective line to `LEARNINGS.md` and emit `HANDOFF:APPROVED`. If you promote a recurring lesson to `AGENTS.md`, also add a deterministic check (lint rule, grep hook, or test) — see AGENTS.md "Enforced lessons" section.
>
> <paste full HANDOFF:VERIFIED block>

Wait for return. Validate. Log.

- `HANDOFF:APPROVED` → report to user with PR URL + retrospective line. **Stop. Do not merge — the user merges manually.**
- `HANDOFF:FIX` → run circuit-breaker check. New signature → loop back to Stage 3 (coder), increment review-fix counter. Repeat signature or counter > 3 → stop and surface.

## When to ask the user

Only ask the user if:

- The planner emits clarifying questions mid-flight.
- A handoff block is malformed (validation fails).
- The red-team pass returns `RED-TEAM:REFUTED`.
- A failure signature repeats.
- The token budget is exceeded.
- A retry counter exceeds 3.

Do not ask for permission at clean stage boundaries — that defeats the purpose of orchestrating.

## What this command does NOT do

- Does **not** merge the PR — the user does that manually after reading `HANDOFF:APPROVED`.
- Does **not** invoke `issue-creator` — backlog setup is a separate flow.
- Does **not** force-push, rebase, or rewrite history.
- Does **not** delete `.pipeline-runs/` — pruning is the user's call.

## Reporting

Throughout the run, surface a short status line at each stage boundary:

```
[run] 20260523-140812-a3f9b1 — budget 400000
[planner] PLAN ready for #<N> on branch task/<N>-<slug>
[red-team] UPHELD — 3 assumptions checked
[coder] PR #<M> opened — 4/4 ACs mapped
[smoke-tester] PASS — 4/4 endpoints, 0 console errors
[reviewer] APPROVED — retrospective appended to LEARNINGS.md
[run] tokens 187340/400000 — log .pipeline-runs/<N>/<run_id>.jsonl
```

When done, paste the final `HANDOFF:APPROVED` block (or the failure summary) and stop.
