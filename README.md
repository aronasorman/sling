# Sling

A minimal human-in-the-loop orchestrator for solo/small-team agentic development.

**Hard rule: no autonomous merging, ever.**

Written in Go. Single binary. Dependencies: `bd`, `jj`, `claude`.

## Workflow

```
sling start LIN-423       # fetch issue, create epic, plan + expand beads
sling next                # claim next ready bead, execute, automated review
sling mail                # see what needs your attention
sling review <bead-id>    # create review commit for human review
sling address <bead-id>   # address-review agent resolves REVIEW: markers
sling done <bead-id>      # squash, push, close bead
```

## Status

Early implementation phase.

## Design

See [docs/PLAN.md](docs/PLAN.md) and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
