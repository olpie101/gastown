# Swarms

A **swarm** is the set of ephemeral worker agents (polecats) simultaneously working
on a convoy's tracked issues. Swarms are not persistent — they have no ID and dissolve
when the work is done.

## Swarm vs Convoy

| | Convoy | Swarm |
|---|---|---|
| **Persistent?** | Yes | No |
| **Has ID?** | Yes (`hq-cv-*`) | No |
| **What it is** | Tracking unit you create, monitor, and get notified about | The workers currently assigned to the convoy's issues |
| **When done** | Lands and stays in history | Dissolves |

When you "kick off a swarm", you're really:
1. Creating a convoy (the persistent tracking unit)
2. Assigning polecats to the tracked issues via `gt sling`
3. The "swarm" is just those polecats while they're working

```
             🚚 Convoy (hq-cv-abc)
                     │
        ┌────────────┼────────────┐
        │            │            │
        ▼            ▼            ▼
   ┌─────────┐  ┌─────────┐  ┌─────────┐
   │ gt-xyz  │  │ gt-def  │  │ bd-abc  │
   │ gastown │  │ gastown │  │  beads  │
   └────┬────┘  └────┬────┘  └────┬────┘
        │            │            │
        ▼            ▼            ▼
   ┌─────────┐  ┌─────────┐  ┌─────────┐
   │  nux    │  │ furiosa │  │  amber  │
   │(polecat)│  │(polecat)│  │(polecat)│
   └─────────┘  └─────────┘  └─────────┘
                     │
                "the swarm"
                (ephemeral)
```

## Swarm Lifecycle

The underlying swarm mechanics (in `internal/swarm/`) track these states:

```
Created ──► Active ──► Merging ──► Landed
                  │           │
                  └──► Failed ◄──┘
                  │
                  └──► Canceled
```

| State | Description |
|-------|-------------|
| `created` | Swarm configured, no work started |
| `active` | Workers actively executing tasks |
| `merging` | All tasks done, branches being merged to integration |
| `landed` | Integration branch merged to target (e.g., main) |
| `failed` | Unrecoverable failure |
| `canceled` | Explicitly canceled by user |

## How Workers Operate

Each polecat in a swarm:
1. Gets assigned a task (a beads issue)
2. Works on its own git branch independently
3. Pushes work to its branch when done
4. The Witness monitors progress, nudges stalled workers, recycles failed ones
5. The Refinery merges completed branches into the integration branch

## Task States

Individual tasks within a swarm progress through:

| State | Meaning |
|-------|---------|
| `pending` | Not yet started |
| `assigned` | Assigned to a polecat but not started |
| `in_progress` | Actively being worked on |
| `review` | Ready for review/merge |
| `merged` | Merged into integration branch |
| `failed` | Task failed |

## Integration Branch

Each swarm uses an integration branch (`swarm/<epic-id>`) as a merge target:
- All workers branch from a shared `BaseCommit`
- Completed task branches merge into the integration branch
- When all tasks are done, the integration branch lands on the target branch (typically `main`)
- After landing, all swarm branches are cleaned up

## Landing Protocol

When a swarm is ready to land (`internal/swarm/landing.go`):
1. Audit all worker git state (no uncommitted or unpushed work)
2. Stop active polecat sessions
3. Merge integration branch to target
4. Clean up all swarm-associated branches
5. Notify subscribers via the convoy

## CLI (Deprecated)

The `gt swarm` commands are deprecated in favor of `gt convoy`:

```bash
# OLD (deprecated)
gt swarm create <rig>
gt swarm status <id>
gt swarm list

# NEW (preferred)
gt convoy create "Feature X" gt-abc gt-def
gt convoy status hq-cv-abc
gt convoy list
```

The swarm package still handles the underlying mechanics (branch management,
integration, landing), but convoys are now the user-facing abstraction.

## See Also

- [Convoys](convoy.md) — The persistent tracking unit
- [Polecat Lifecycle](polecat-lifecycle.md) — How workers are managed
- [Overview](../overview.md) — Gas Town architecture
