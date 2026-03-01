# Sling Implementation Plan

Full design doc iterated with Opus: https://claude.ai/public/artifacts/b3d329c0-ba02-49a0-9afb-043878e97295

## Phases

- **Phase 0:** Scaffolding — cobra CLI, config loading, bd/jj wrappers
- **Phase 1:** Intake — `sling start` fetches Linear/GitHub issue, creates epic bead
- **Phase 2:** Planning — Opus decomposes epic into child beads with DAG
- **Phase 3:** Expansion — bd formulas add sub-task checklists, beads → sling:ready
- **Phase 4:** Execution — `sling next` spawns Sonnet in jj worktree, up to 3 attempts
- **Phase 5:** Automated review — Sonnet reviews diff, address-review loop until clean
- **Phase 6:** Human review — `sling review`, `sling address`, `sling done`
- **Phase 7:** Notifications — Telegram pings on review-pending, failed, blocked

## State model

| Sling stage     | bd status   | Label                |
|-----------------|-------------|----------------------|
| Planned         | open        | sling:planned        |
| Ready           | open        | sling:ready          |
| Executing       | in_progress | sling:executing      |
| Review pending  | in_progress | sling:review-pending |
| Addressing      | in_progress | sling:addressing     |
| Failed          | blocked     | sling:failed         |
| Blocked         | blocked     | sling:blocked        |
| Done            | closed      | (none)               |

## Key decisions

- Claude Code SDK: github.com/yukifoo/claude-code-sdk-go
- GitHub issues: github.com/google/go-github/v68
- Linear: hand-rolled GraphQL client (net/http)
- ADO: out of scope for v1 (v1 = GitHub only)
- Config: cobra + viper, sling.toml per-repo + ~/.config/sling/config.toml global
- No autonomous merging: agent pushes branch, opens PR, stops
- Executor signals completion by creating .sling-done in worktree root
- Planner writes JSON plan to /tmp/sling-plan.json (not parsed from stdout)
