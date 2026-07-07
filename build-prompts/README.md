# Build prompts — how to run these

Six phases, **strictly sequential** — each builds on the previous one's code.
Do **not** run them in parallel: they share `go.mod`, the migration file, and
(later) the single-file PWA, so parallel agents would collide constantly.

## Recommended dispatch: one fresh Claude Code session per phase

Sonnet-class models handle these fine because the design decisions are already
made — the prompts are execution work, not architecture work.

```
cd ~/projects/food
claude --model sonnet
> Read CLAUDE.md, ARCHITECTURE.md, and build-prompts/phase-1-scaffold.md, then execute phase 1.
```

Then, **in a new session for each subsequent phase** (fresh context beats a
long polluted one):

```
> Read CLAUDE.md, ARCHITECTURE.md, and build-prompts/phase-N-*.md, then execute phase N.
```

## Gate between phases (do this yourself, don't skip)

1. `go build ./... && go test ./...` must pass.
2. Run the phase's **Acceptance checklist** (bottom of each prompt file).
3. `git add -A && git commit -m "Phase N: <name>"` — a clean commit per phase
   means a botched phase is one `git reset --hard` away from retry.
4. If a phase went sideways, reset and re-run it in a fresh session with a
   note about what went wrong appended to your kickoff message. Don't patch a
   confused session — restart it.

## Alternative: dispatched agents from one orchestrating session

You can instead open one Claude Code session and ask it to run each phase via
the Agent tool (`subagent_type: general-purpose`, `model: sonnet`), verifying
and committing between phases itself. This works, but you lose the ability to
eyeball each phase before it's built upon. Recommended only for phases 1–2
(mechanical scaffolding); drive 3–6 interactively since AI-pipeline and PWA
choices benefit from your taste ("that preview card is too busy") in the loop.

## Phase map

| Phase | File | Delivers |
|---|---|---|
| 1 | phase-1-scaffold.md | Go server skeleton, DB schema, auth, Dockerfile |
| 2 | phase-2-core-api.md | Meals/weights/settings CRUD, summaries, score math + tests |
| 3 | phase-3-ai-pipeline.md | Claude parse loop, USDA + Open Food Facts clients |
| 4 | phase-4-photos.md | R2 upload, vision analysis, label/plate/barcode paths |
| 5 | phase-5-pwa.md | The entire PWA: Today, log flow, Trends, Settings |
| 6 | phase-6-polish-deploy.md | Export, smoke script, README, Render deploy |

Nothing in phases 1–4 requires the PWA; test with `curl`. Phase 5 is the
biggest single prompt — budget a long session for it.
