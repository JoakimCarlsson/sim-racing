# Learnings

One line per merged PR. Appended by the Reviewer agent at the end of every review (CLEAN or otherwise) as the **Retrospective** step in the pipeline.

The point: each PR teaches something about how the pipeline went wrong (Coder ignored a convention, Planner missed a file, Reviewer rubber-stamped something) — or what would have prevented a re-run if it had been documented in `AGENTS.md` from the start. Capturing that here means the same mistake costs at most one cycle.

When an entry below appears 2+ times, promote it to `AGENTS.md` (or the relevant `docs/` page) so the agents read it as a hard rule next time.

## Entries

<!-- Format: `- YYYY-MM-DD #<PR>: <one short sentence>` -->
<!-- Reviewer appends here. Most recent at the bottom. -->- 2026-05-24 #52: Bootstrap PRs with no CI workflows will show 'no checks reported' — reviewer should proceed when zero workflows exist rather than waiting indefinitely; add a GitHub Actions CI workflow (go build/vet/test) as the very next scaffolding issue.
- 2026-05-24 #53: Frontend-only PRs with no HTTP routes have no E2E curl requirement; the HANDOFF:VERIFIED schema's verification[] browser-smoke entry is still required — smoke-tester should note canvas injection and console-errors count even when Playwright is unavailable.
- 2026-05-24 #54: coder/websocket returns 426 (not 400) for non-WS requests — inline comment in realtime_endpoint.go said '400/403' but actual status is 426; always verify status codes by testing rather than citing library docs in comments.
- 2026-05-24 #56: Pure package PRs with no HTTP routes need no curl evidence, but PR body test-plan citations for vet/test/import ACs cannot point to a file:line — document that tooling-verified ACs should cite the CI run URL or the relevant source file rather than 'verified locally'.
- 2026-05-24 #57: Physics ACs that involve AGENTS.md hard rules (TestReplayDeterminism golden + WASM parity) not yet implemented in the repo should be tracked as a follow-up issue rather than silently skipped; planner should explicitly mark them as non-goals when the harness does not exist yet.
- 2026-05-24 #58: Lateral-dynamics PRs must use vTotal (abs forward speed) as the bicycle-model speed denominator, not raw LinearVel[2], to avoid sign inversion when reversing; using abs32(vNew) before slip-angle computation prevents divide-by-sign errors without extra branches.
- 2026-05-24 #59: State.Grounded was declared in the struct per the issue spec but never populated in step.go; when a new State field has semantic meaning, the HANDOFF:PLAN must include an explicit AC for populating it, or explicitly mark it a non-goal.
- 2026-05-24 #60: PR-body test-plan citations that point to a non-existent section (e.g. 'environment_notes') rather than a CI URL or file:line slip past the coder; reviewer must reject any AC evidence reference that is not navigable to a concrete artifact.
- 2026-05-24 #61: The issue AC1 size target (<500 KB) was a soft goal overridden by the TinyGo rejection decision; planner should explicitly list the size cap override in non_goals[] so reviewers do not have to excavate docs and pipeline-run notes to confirm the deviation was intentional.
- 2026-05-24 #63: Pure library PRs (no HTTP routes) need no curl evidence; TestZeroAllocs intentionally omits ServerHello Unmarshal because GearRatios must alloc on first decode — document hot-path vs. connection-setup alloc expectations in the issue AC when the distinction matters.
- 2026-05-24 #64: Rate-limit check fires before seq-monotonicity in validateInput — ordering is intentional (flood defence first) but must be documented in the issue AC or non_goals when evaluation order affects which InvalidReason gets logged for a fast-replayed-seq packet.
- 2026-05-24 #65: Issue AC1 said tick-rate tolerance ±2%/10s but test used ±5%/1.5s — planner should standardise tick-rate AC wording to ±5%/1.5s (CI-safe) or explicitly note that the ±2%/10s requirement is a runtime SLO, not a unit-test bound, to avoid spec-drift confusion in future reviews.
- 2026-05-24 #66: atomic.Bool closed-guard on SendSnapshot stops channel-send-after-close panics, but the test only exercises the post-teardown path — a true concurrent-with-teardown stress test would need a barrier to trigger the narrow TOCTOU window
- 2026-05-24 #67: InputSender.start()-on-WS-open (not ServerHello) is a deliberate deviation from the issue spec — such architecture decisions must be captured in HANDOFF:PLAN non_goals[] or assumptions[] so the reviewer does not have to excavate the PR body to confirm the deviation was intentional.
- 2026-05-24 #68: Client prediction PRs that lack Playwright should prove cube motion via a WASM parity unit test rather than a browser screenshot — document this as the accepted substitute evidence for animation-loop ACs in AGENTS.md.
- 2026-05-24 #69: PR body line citations to test cases are fragile across iterations — reviewer re-runs caused solely by stale line numbers; planner should instruct the coder to anchor test-plan citations to describe/test names rather than bare line numbers so they survive edits without a reviewer FIX cycle.
