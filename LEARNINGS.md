# Learnings

One line per merged PR. Appended by the Reviewer agent at the end of every review (CLEAN or otherwise) as the **Retrospective** step in the pipeline.

The point: each PR teaches something about how the pipeline went wrong (Coder ignored a convention, Planner missed a file, Reviewer rubber-stamped something) — or what would have prevented a re-run if it had been documented in `AGENTS.md` from the start. Capturing that here means the same mistake costs at most one cycle.

When an entry below appears 2+ times, promote it to `AGENTS.md` (or the relevant `docs/` page) so the agents read it as a hard rule next time.

## Entries

<!-- Format: `- YYYY-MM-DD #<PR>: <one short sentence>` -->
<!-- Reviewer appends here. Most recent at the bottom. -->- 2026-05-24 #52: Bootstrap PRs with no CI workflows will show 'no checks reported' — reviewer should proceed when zero workflows exist rather than waiting indefinitely; add a GitHub Actions CI workflow (go build/vet/test) as the very next scaffolding issue.
- 2026-05-24 #53: Frontend-only PRs with no HTTP routes have no E2E curl requirement; the HANDOFF:VERIFIED schema's verification[] browser-smoke entry is still required — smoke-tester should note canvas injection and console-errors count even when Playwright is unavailable.
