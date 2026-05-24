---
name: github-issues-labels
description: >-
  Creates and structures GitHub issues and labels using gh CLI conventions. Use
  when creating or editing GitHub issues, triaging, defining label taxonomy, or
  running gh issue or gh label commands. Use when the user asks to create issues,
  plan a feature with issues, break down work into GitHub tasks, or set up labels
  and milestones.
---

# GitHub Issues and Labels

## Tools

Use **`gh`** for GitHub (issues, labels, milestones, PRs). Use **`git`** only for local repo operations. Do not use the web UI for bulk or scripted tasks unless the user asks.

---

## Core Principles

### Every issue must leave the codebase in a runnable state

- Never split work so the project is broken mid-series. Each issue is self-contained or extends something already working.
- Always include **How to verify (runnable)** ÔÇö exact local steps before opening a PR.
- **Acceptance criteria** must be binary (done or not done).

### Every issue must explain why, not just what

1. **Why** ÔÇö motivation, user story, constraint
2. **What** ÔÇö precise scope
3. **How** ÔÇö design decisions, edge cases, contracts
4. **Done when** ÔÇö runnable verification before PR

---

## Issue Body Template

```markdown
## Why / Context

## What needs to happen

## How it should work

## Acceptance criteria
- [ ] Specific, binary, testable criterion

## How to verify (runnable)

## Dependencies / notes
```

---

## Labels

**Every issue gets at least one label.** One value per axis when that axis carries signal.

### `prefix/value` axes

| Prefix | Vocabulary | Question |
|--------|------------|----------|
| `kind/` | closed | Type of work |
| `priority/` | closed | Urgency + timeframe |
| `triage/` | closed | Triage state |
| `lifecycle/` | closed | Active vs stale |
| `area/` | open ÔÇö from this codebase | Subsystem |
| `sig/` or `team/` | open ÔÇö from org | Owner team |

**`kind/`:** `kind/bug`, `kind/feature`, `kind/cleanup`, `kind/refactor`, `kind/documentation`, `kind/security`, etc. Use `kind/feature` ÔÇö never bare `enhancement`.

**`priority/`:** `priority/critical-urgent`, `priority/important-soon`, `priority/important-longterm`, `priority/backlog`, `priority/awaiting-more-evidence`. Never bare `high`/`medium`/`low`.

**`triage/`:** `triage/accepted`, `triage/needs-information`, etc.

**`area/`:** derive from directory/module boundaries in **this** repo ÔÇö never copy from another project.

**Unprefixed (process):** `good-first-issue`, `help-wanted`, `wontfix`, etc.

Skip an axis if it does not vary across the batch (e.g. skip `priority/*` when every bootstrap issue is equally urgent).

```bash
gh label list
gh label create "<prefix/value>" --description "<short>" --color "<hex without #>"
gh issue edit <num> --add-label "kind/feature,area/api"
```

---

## Series ordering

Determine order before writing bodies. Encode dependencies in **Dependencies / notes** ÔÇö not `blocked-by` / `blocks` labels.

For a series: create issues first, then second pass to add `#N` cross-references. Optional tracking issue with checklist.

---

## Milestones

```bash
# List
gh api "repos/$(gh repo view --json nameWithOwner -q .nameWithOwner)/milestones"

# Create
gh api "repos/$(gh repo view --json nameWithOwner -q .nameWithOwner)/milestones" \
  -f title="Milestone title" -f description="Goal" -f state="open"

# Assign issue
gh issue edit <num> --milestone "Milestone title"
```

---

## Creating issues

```bash
gh issue create --title "Clear, specific title" --body-file <path> --label "kind/feature,area/core"
```

- On **PowerShell**, always use `--body-file` (no bash heredocs).
- Do not pass `--label enhancement` reflexively.
- Second pass: `gh issue edit <num> --body-file <path>` for cross-refs after all numbers exist.

---

## Do Not

- Vague acceptance criteria or missing **How to verify**
- Issues that break the build mid-series
- Bare `enhancement`, `high`/`low`, dependency labels (`blocked-by`, `blocks`)
- Invented prefixes without a real axis
- AI/Cursor `Co-authored-by` on commits
