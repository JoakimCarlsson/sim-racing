# Pipeline observability

Each `/pipeline` run (or its Cursor/VS Code equivalent) writes a JSON-Lines log so we can see which stages cost what and where failures cluster. The log lives at:

```
.pipeline-runs/<issue-number>/<run-id>.jsonl
```

`<run-id>` is `YYYYMMDD-HHMMSS-<6-hex>` (UTC). The `.pipeline-runs/` directory is gitignored; aggregated stats can be committed separately.

## Row schema (one row per stage invocation)

```json
{
  "schema_version": "1",
  "ts": "2026-05-23T14:08:12Z",
  "run_id": "20260523-140812-a3f9b1",
  "issue_number": 49,
  "pr_url": "https://github.com/.../pull/123",
  "stage": "planner",
  "agent": "planner",
  "model": "claude-opus-4-7",
  "attempt": 1,
  "tokens_in": 14820,
  "tokens_out": 4210,
  "duration_ms": 92140,
  "verdict": "HANDOFF:PLAN",
  "failure_signature": null,
  "notes": "branch created, plan written"
}
```

| Field               | Type    | Notes |
|---------------------|---------|-------|
| `schema_version`    | string  | `"1"` |
| `ts`                | string  | ISO-8601 UTC, stage end |
| `run_id`            | string  | unique per `/pipeline` invocation |
| `issue_number`      | integer | from the handoff envelope |
| `pr_url`            | string? | null before the coder opens the PR |
| `stage`             | enum    | `planner` / `red-team` / `coder` / `smoke-tester` / `reviewer` |
| `agent`             | string  | subagent type or label (e.g. `red-team`) |
| `model`             | string  | concrete model id |
| `attempt`           | integer | 1 on first call, 2+ on retries |
| `tokens_in`         | integer | best-effort from the Agent tool result |
| `tokens_out`        | integer | best-effort from the Agent tool result |
| `duration_ms`       | integer | wall time spent in this stage |
| `verdict`           | string  | the `HANDOFF:*` type emitted, or `ERROR` |
| `failure_signature` | string? | 12-char hash from `docs/pipeline-handoff-schema.md` |
| `notes`             | string? | one-line free text |

A stage that escalates to the user (malformed handoff, repeat signature, budget exceeded) is still logged with `verdict: "ERROR"` and a `notes:` line saying why.

## Aggregation

The log is plain JSONL — read it with whatever you have lying around. Examples:

```bash
# Total tokens per run
jq -s 'group_by(.run_id) | map({run: .[0].run_id, tokens: (map(.tokens_in + .tokens_out) | add)})' \
  .pipeline-runs/<issue>/*.jsonl

# Repeating failure signatures across the last 20 runs
ls -t .pipeline-runs/**/*.jsonl | head -20 | xargs cat | \
  jq -r 'select(.failure_signature) | "\(.failure_signature.stage)|\(.failure_signature.class)|\(.failure_signature.symbol)"' | \
  sort | uniq -c | sort -rn
```

There is no dedicated aggregator script — if you find yourself running the same query often, add it to the docs, not to `scripts/`.

## Retention

`.pipeline-runs/` is local-only and gitignored. Prune by age with:

```bash
find .pipeline-runs -type d -mtime +30 -exec rm -rf {} +
```

There is no automated retention job — keep what you need for retrospectives, drop the rest.
