---
description: Loop the full delivery pipeline (planner → red-team → coder → smoke-tester → reviewer → merge) over every open issue in a milestone. Auto-merges each PR (squash + delete branch), then picks the next issue until the milestone is empty.
argument-hint: <milestone-title>
---

# /pipeline — milestone loop

You are the **pipeline orchestrator**. The argument is a **milestone title**, not an issue number. Loop:

1. Pick the next open issue in the milestone.
2. Run planner → red-team → coder → smoke-tester → reviewer.
3. On `HANDOFF:APPROVED`, merge the PR (squash + delete branch).
4. Go back to step 1.
5. Stop when the milestone has zero open issues.

Milestone for this run: **$ARGUMENTS**

## How chaining works in Claude Code

`.cursor/agents/` and `.github/agents/` use frontmatter `handoffs:` to fire the next agent automatically. Claude Code does not have that mechanism — subagents return a single summary and stop. This slash command bridges the gap.

Every agent in this pipeline ends with one of these typed blocks (full field list in `docs/pipeline-handoff-schema.md`):

- `HANDOFF:PLAN` (planner → red-team → coder)
- `HANDOFF:IMPLEMENTATION` (coder → smoke-tester)
- `HANDOFF:VERIFIED` (smoke-tester → reviewer)
- `HANDOFF:FIX` (smoke-tester or reviewer → coder)
- `HANDOFF:APPROVED` (reviewer → merge step)

Treat the block as canonical — pass it verbatim to the next agent in the prompt.

## Run setup (do this once at the very start)

1. Resolve the milestone:
   ```bash
   REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
   gh api "repos/$REPO/milestones" --jq '.[] | select(.state=="open") | {title, number, open_issues}'
   ```
   Match `$ARGUMENTS` against `title` (case-insensitive). If not found, stop and surface the list of open milestones.
2. Pick a `run_id`: `YYYYMMDD-HHMMSS-<6-hex>` (UTC). One `run_id` for the whole milestone loop.
3. Create the log directory: `.pipeline-runs/milestone-<slug>/` where `<slug>` is the milestone title lowercased with non-alnum→`-`.
4. Open `run.jsonl` at `.pipeline-runs/milestone-<slug>/<run_id>.jsonl`. Every stage of every issue logs here (include `issue_number` in each row).
5. Initialise counters in memory: `tokens_total = 0`, per-issue per-stage `attempt = 0`, per-issue `signatures_seen = {}`, `issues_completed = []`.
6. Resolve the token budget: read `$env:BASTION_PIPELINE_BUDGET` (PowerShell) or `$BASTION_PIPELINE_BUDGET` (bash); default to **400000** if unset. **Budget applies across the entire milestone loop**, not per issue.

## Outer loop — pick next issue

At the top of every iteration:

```bash
gh issue list --milestone "$ARGUMENTS" --state open \
  --json number,title,labels,url \
  --jq 'sort_by((.labels|map(.name)|map(select(test("^priority:")))|.[0]//"priority:zzz"), .number)'
```

- If the list is empty → **milestone done**. Print summary (issues completed, PRs merged, tokens used, log path) and stop.
- Else take the first issue. Set `ISSUE_NUMBER` and `ISSUE_URL`. Reset per-issue counters. Proceed to Stage 1.

Skip issues that are assigned to someone else or labeled `blocked` / `wontfix` / `needs-triage`. Log a skip row.

## Validation contract (apply at every stage boundary)

Before invoking the next stage, run these checks against the block returned by the prior stage:

1. **Envelope:** the block is fenced by `---HANDOFF:<TYPE>---` and `---END HANDOFF---` on their own lines.
2. **YAML:** the body parses as YAML.
3. **Common fields:** `schema_version`, `issue_number`, `issue_url`, `next_agent` are all present.
4. **Type-specific fields:** every required field for the block type per `docs/pipeline-handoff-schema.md` is present and non-empty.
5. **Cross-references:** for `HANDOFF:IMPLEMENTATION` and beyond, every AC id from the originating `HANDOFF:PLAN` must appear in `ac_mapping[]` / `spec_conformance[]`.

**Malformed = stop the entire milestone loop and surface.** Never invoke the next LLM stage — and never advance to the next issue — on a malformed handoff. Log the row with `verdict: "ERROR"`.

## Failure-signature circuit breaker (per issue)

Every `HANDOFF:FIX` carries a `failure_signature: { stage, class, symbol }`. Compute:

```
hash = sha1(stage + "|" + class + "|" + symbol)[:12]
```

Maintain `signatures_seen` **per issue** (reset between issues). If the hash repeats within the same issue, **stop the entire milestone loop** and surface — do not skip the issue and move on, because a repeating signature usually means an architectural problem worth a human look.

Per-stage `attempt` is hard-capped at 3 per issue as a backstop.

## Token-budget ceiling

After each stage returns, add `tokens_in + tokens_out` to `tokens_total`. If `tokens_total > budget`, stop the milestone loop and surface. Do not start a new issue if the budget is already over.

## Logging

After every Agent invocation and every merge attempt, append one JSON line to the run log. Include `issue_number`, `stage`, `attempt`, `verdict`, `tokens_in`, `tokens_out`, `notes`. Use PowerShell `Add-Content -Path <log> -Value "<json>" -Encoding utf8` on Windows, or `>>` on POSIX.

## Per-issue sequence

### Stage 1 — planner

Invoke `Agent` with `subagent_type: planner` and the prompt:

> Run the planner workflow for issue #<ISSUE_NUMBER> in milestone "$ARGUMENTS". Read AGENTS.md, .claude/agents/_bastion-conventions.md, docs/backend-architecture.md, docs/pipeline-handoff-schema.md, and LEARNINGS.md before drafting. Create the linked task branch via `gh issue develop`. Produce a **structured** plan per the schema (acceptance_criteria with ids, files_touched, interfaces, test_cases, non_goals, assumptions). End your response with the full `HANDOFF:PLAN` block.

Validate. Log.

### Stage 2 — red-team

Invoke `Agent` with `subagent_type: red-team` and the prompt:

> You are the **red-team** for this pipeline run. Walk every entry in `assumptions[]` from the plan below and attempt to refute it by reading the repo. For each assumption, run the `refutable_by` command (or grep/read the cited file) and report:
>
> - `id: <A1>` — UPHELD or REFUTED (with the file:line or command output that contradicts it)
>
> Do not write code. Do not invoke other agents. End with a single line: `RED-TEAM:UPHELD` or `RED-TEAM:REFUTED` plus a one-paragraph summary.
>
> <paste full HANDOFF:PLAN block>

If `RED-TEAM:REFUTED`, **stop the milestone loop** and surface — a wrong premise needs a human, not a different issue. Log the row.

If `RED-TEAM:UPHELD`, proceed to Stage 3.

### Stage 3 — coder

Invoke `Agent` with `subagent_type: coder` and the prompt:

> Implement the plan below. **Refuse to start** if any `acceptance_criteria[].id` is missing from `test_cases[]` — emit a short `HANDOFF:FIX` back to planner instead. Tests-first for any change under `internal/<subsystem>/`. End with the full `HANDOFF:IMPLEMENTATION` block including the PR URL, `ac_mapping[]`, and `drift_log[]`.
>
> <paste full HANDOFF:PLAN block>

Validate. Log.

### Stage 4 — smoke-tester

Invoke `Agent` with `subagent_type: smoke-tester` and the prompt:

> Run smoke tests against the implementation below. Observer/recorder only — no fixes. End with `HANDOFF:VERIFIED` on pass or `HANDOFF:FIX` on fail. Every `HANDOFF:FIX` must include `failure_signature: { stage, class, symbol }`.
>
> <paste full HANDOFF:IMPLEMENTATION block>

Validate. Log.

- `HANDOFF:VERIFIED` → Stage 5.
- `HANDOFF:FIX` → circuit-breaker check. New signature → loop to Stage 3 with the FIX block, increment smoke-fix counter (cap 3). Repeat signature or counter > 3 → **stop milestone loop**.

### Stage 5 — reviewer

Invoke `Agent` with `subagent_type: reviewer` and the prompt:

> Review PR <pr_url>. Wait for CI green before reading the diff. Run the spec-conformance pass (cite `file:line` per AC or mark UNMET). Any `HANDOFF:FIX` must include `failure_signature: { stage: reviewer, class, symbol }`. On CLEAN, append the retrospective line to `LEARNINGS.md` and emit `HANDOFF:APPROVED`.
>
> <paste full HANDOFF:VERIFIED block>

Validate. Log.

- `HANDOFF:APPROVED` → proceed to Stage 6 (merge).
- `HANDOFF:FIX` → circuit-breaker check. New signature → loop to Stage 3, increment review-fix counter (cap 3). Repeat signature or counter > 3 → **stop milestone loop**.

### Stage 6 — merge

The PR URL is in the `HANDOFF:APPROVED` block. Merge it:

```bash
PR_URL=<pr_url from HANDOFF:APPROVED>
gh pr view "$PR_URL" --json mergeStateStatus,state,statusCheckRollup
# Sanity check: state == OPEN, mergeStateStatus in {CLEAN, UNSTABLE} (UNSTABLE = non-required check failing — still mergeable)
gh pr merge "$PR_URL" --squash --delete-branch
```

If `gh pr merge` fails (merge conflict, required checks failing, branch protection block), **stop the milestone loop** and surface the `gh` output. Do not retry; do not skip to the next issue. The reviewer signed off — a merge failure here is unexpected and needs eyes.

On success:
- Append `issues_completed += [ISSUE_NUMBER]`.
- Pull the updated default branch locally so the next planner iteration starts clean:
  ```bash
  DEFAULT=$(gh repo view --json defaultBranchRef -q .defaultBranchRef.name)
  git switch "$DEFAULT" && git pull --ff-only origin "$DEFAULT"
  ```
- Log a `stage: "merge"` row with `verdict: "MERGED"`.
- Print the boundary line (see Reporting) and return to the **Outer loop**.

## When to ask the user

Only ask if:

- Planner emits clarifying questions mid-flight.
- A handoff block is malformed.
- Red-team returns `RED-TEAM:REFUTED`.
- A failure signature repeats within an issue.
- Token budget exceeded.
- A retry counter exceeds 3.
- `gh pr merge` fails.
- The milestone title cannot be resolved.

No permission prompts at clean stage boundaries — that defeats the orchestration.

## What this command does NOT do

- Does **not** force-push, rebase, or rewrite history.
- Does **not** merge PRs from issues outside the named milestone.
- Does **not** invoke `issue-creator` — backlog setup is a separate flow.
- Does **not** delete `.pipeline-runs/` — pruning is the user's call.
- Does **not** retry a failed merge automatically.

## Reporting

At each stage boundary, surface a short status line. Issue-scoped lines are prefixed with `[#<N>]`:

```
[run] 20260524-093012-a3f9b1 — milestone "Physics MVP" — 7 open issues — budget 400000
[#62] [planner] PLAN ready on branch task/62-replay-ui
[#62] [red-team] UPHELD — 3 assumptions checked
[#62] [coder] PR #74 opened — 4/4 ACs mapped
[#62] [smoke-tester] PASS — 4/4 endpoints, 0 console errors
[#62] [reviewer] APPROVED — retrospective appended to LEARNINGS.md
[#62] [merge] MERGED via squash, branch deleted — main fast-forwarded
[run] tokens 187340/400000 — 1/7 issues complete — next: #63
...
[run] DONE — milestone "Physics MVP" empty — 7 issues merged — log .pipeline-runs/milestone-physics-mvp/<run_id>.jsonl
```

When the milestone is empty (or the loop stops for any of the reasons above), paste a final summary block:

```
issues_merged: [62, 63, 64, ...]
issues_skipped: [...]
tokens_used: <n>/<budget>
stopped_reason: milestone_empty | red_team_refuted | repeat_signature | budget_exceeded | merge_failed | malformed_handoff | retry_cap | planner_question
log: .pipeline-runs/milestone-<slug>/<run_id>.jsonl
```

and stop.
