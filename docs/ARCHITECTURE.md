# Sling Architecture

## Pipeline

```
sling start <issue>
  ├─ Intake:    fetch issue → create epic bead (bd)
  ├─ Planning:  Opus decomposes → child beads with DAG (writes /tmp/sling-plan.json)
  └─ Expansion: bd formulas → sub-task checklists, label sling:ready

sling next
  ├─ Claim next sling:ready bead (check DAG deps all closed)
  ├─ Create jj worktree at <repo>/.sling-worktrees/<bead-id>/
  ├─ Executor (Sonnet): implement + tests, up to max_attempts
  │    └─ Done: agent creates .sling-done in worktree root
  │    └─ Failure → sling:failed, Telegram ping
  ├─ Automated review (Sonnet): REVIEW: markers + address loop (max 3 rounds)
  └─ Label sling:review-pending, Telegram ping

sling review <id>    → human adds REVIEW: markers in code + review commit message
sling address <id>   → Addresser (Sonnet) uses address-review skill
sling done <id>      → jj squash + git push branch + bead closed
```

## Agent roles

| Role      | Model  | Purpose                               |
|-----------|--------|---------------------------------------|
| Planner   | Opus   | Decompose epic into atomic beads      |
| Executor  | Sonnet | Implement a single bead               |
| Reviewer  | Sonnet | Adversarial: find problems in diff    |
| Addresser | Sonnet | Constructive: resolve REVIEW: markers |

Reviewer never addresses its own feedback. Each role = separate Claude Code session.

## Project structure

```
sling/
├── cmd/sling/main.go
├── internal/
│   ├── config/       # sling.toml + ~/.config/sling/config.toml
│   ├── issue/        # IssueSource interface, GitHub + Linear fetchers
│   ├── agent/        # Claude Code wrapper, roles, system prompts
│   ├── bead/         # bd CLI wrapper (create, show, update, label, list)
│   ├── worktree/     # jj CLI wrapper (workspace add/remove, branch, squash, push)
│   ├── pipeline/     # intake, plan, expand, execute, review, human
│   └── notify/       # Telegram
├── docs/
├── sling.toml.example
└── go.mod
```

## Review commit done condition

No REVIEW: markers remain in any file (grep, not diff check) AND commit message is empty/whitespace.
Marker formats: `# REVIEW:`, `// REVIEW:`, `-- REVIEW:`, `<!-- REVIEW:`, `/* REVIEW:`
