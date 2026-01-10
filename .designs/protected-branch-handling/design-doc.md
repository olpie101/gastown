# Design: Refinery Protected Branch Handling

## Executive Summary

The Gas Town refinery currently attempts direct pushes to `main` after merging polecat branches. This fails silently when the target branch has GitHub branch protection rules enabled (required reviews, status checks, etc.).

This design proposes adding protected branch detection to the refinery patrol flow. When protection is detected, the refinery will create a GitHub PR instead of direct pushing, leveraging the existing gate system to handle the async approval workflow.

The implementation uses the existing `gh` CLI for GitHub API access, adds minimal new data structures (a protection cache and new failure type), and integrates cleanly with the existing Deacon gate system for PR status polling.

## Problem Statement

**Current behavior**: The refinery's `merge-push` step does `git push origin main`, which fails with an error when the branch is protected. The failure is treated as a generic push failure (`FailurePushFail`) with no special handling.

**Desired behavior**: The refinery should detect protected branches before pushing and route to an appropriate workflow (PR-based merge) when protection is enabled.

**Downstream impact**: This research informs implementation tasks gt-dkx (detection), gt-0ae (workflow), and gt-0ug (gate integration).

## Proposed Design

### Overview

1. **Detection**: Before pushing, query GitHub API for branch protection status
2. **Caching**: Store protection status in memory to avoid repeated API calls
3. **Routing**: If protected, create PR instead of direct push
4. **Async handling**: Use gate system to wait for PR checks/approval

```
                                    ┌─────────────────┐
                                    │ Check Protection│
                                    │   (cached)      │
                                    └────────┬────────┘
                                             │
                              ┌──────────────┴──────────────┐
                              │                             │
                              ▼                             ▼
                    ┌─────────────────┐           ┌─────────────────┐
                    │  Not Protected  │           │    Protected    │
                    │  → Direct Push  │           │   → Create PR   │
                    └─────────────────┘           └────────┬────────┘
                                                           │
                                                           ▼
                                                  ┌─────────────────┐
                                                  │  Create Gate    │
                                                  │  gh:pr:<number> │
                                                  └────────┬────────┘
                                                           │
                                                           ▼
                                                  ┌─────────────────┐
                                                  │ Deacon Polls    │
                                                  │ (existing flow) │
                                                  └────────┬────────┘
                                                           │
                                                           ▼
                                                  ┌─────────────────┐
                                                  │ Merge PR        │
                                                  │ (gh pr merge)   │
                                                  └─────────────────┘
```

### Key Components

#### 1. Protection Cache (Data)

```go
// internal/refinery/protection.go
type BranchProtection struct {
    Branch              string
    IsProtected         bool
    RequiresPullRequest bool
    RequiredReviewers   int
    RequiredChecks      []string
    CheckedAt           time.Time
}

type ProtectionCache struct {
    mu    sync.RWMutex
    cache map[string]*BranchProtection  // key: "owner/repo:branch"
    ttl   time.Duration                 // default: 1 hour
}
```

#### 2. Protection Detection (API)

```go
// internal/refinery/github.go
func (g *GitHub) CheckBranchProtection(owner, repo, branch string) (*BranchProtection, error) {
    // Use gh CLI for consistency with existing code
    cmd := exec.Command("gh", "api",
        fmt.Sprintf("repos/%s/%s/branches/%s/protection", owner, repo, branch),
        "--silent")

    output, err := cmd.Output()
    if err != nil {
        // 404 = not protected, 403 = no permission (assume protected)
        if exitErr, ok := err.(*exec.ExitError); ok {
            if exitErr.ExitCode() == 1 {
                return &BranchProtection{IsProtected: false}, nil
            }
        }
        // Assume protected on permission errors (fail safe)
        return &BranchProtection{IsProtected: true}, nil
    }

    // Parse protection rules from JSON...
}
```

#### 3. PR Workflow (Integration)

Modified `merge-push` step in patrol formula:
```toml
# Before pushing, check protection
# If protected:
gh pr create --base main --head temp --title "Merge <issue-id>" --body "..."
bd gate create --await gh:pr:<pr-number> --waiter <refinery-mail>
# Park and continue to next MR; gate system handles async

# When gate clears (Deacon dispatch):
gh pr merge <pr-number> --merge --delete-branch
# Continue with MERGED notification as normal
```

### Interface

**New failure type**:
```go
const FailureProtectedBranch FailureType = "protected_branch"
```

**New CLI output** (when protection detected):
```
Protected branch detected: main
Creating PR instead of direct push...
PR #123 created, waiting for required checks
Gate gh:pr:123 created, parking merge
```

### Data Model

| Type | Purpose | Persistence |
|------|---------|-------------|
| `BranchProtection` | Cache entry for a branch | In-memory, TTL 1 hour |
| `ProtectionCache` | Map of branch → protection | In-memory, session lifetime |
| `FailureProtectedBranch` | New failure type | N/A (enum value) |

## Trade-offs and Decisions

### Decisions Made

| Decision | Rationale |
|----------|-----------|
| Use `gh` CLI | Consistent with existing codebase, handles auth |
| In-memory cache | Protection rules rarely change, simple implementation |
| Pre-push detection | Cleaner UX than catch-and-retry |
| Fail-safe on 403 | Assume protected when permission denied |
| Gate-based async | Leverages existing Deacon infrastructure |

### Open Questions

1. **Required reviewers**: Should we auto-request reviews, or leave for humans?
   - *Recommendation*: Leave for humans in MVP; auto-request in Phase 2

2. **Merge method**: Which merge method (merge, squash, rebase)?
   - *Recommendation*: Use `--merge` to match current `--no-ff` behavior

3. **Scope prompt**: Should we prompt users to add `administration:read` scope?
   - *Recommendation*: Yes, with `gh auth refresh -s admin:read` instructions

4. **Config override**: Allow users to manually mark branches as protected?
   - *Recommendation*: Not for MVP; auto-detection is sufficient

### Trade-offs

| Trade-off | Chosen | Alternative | Reason |
|-----------|--------|-------------|--------|
| API call per merge vs config | API call | User config | Auto-detection is accurate |
| In-memory vs disk cache | In-memory | Disk | Simple, protection rarely changes |
| Pre-detect vs catch-retry | Pre-detect | Catch-retry | Cleaner UX |

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Token lacks admin:read scope | Protection check fails | Fail-safe to PR workflow |
| Rate limiting on busy repos | Slowed merges | Cache with 1-hour TTL |
| SSO re-auth required | Blocked until user re-auths | Clear error message with instructions |
| PR requires human review | Merge delayed | Document expected workflow |
| Deacon not running | Gate never clears | Existing Deacon health checks |

## Implementation Plan

### Phase 1: Detection (gt-dkx)
- Add `BranchProtection` type and cache
- Add `CheckBranchProtection()` function using `gh api`
- Add `FailureProtectedBranch` failure type
- Modify `doMerge()` to check protection before push
- On protected branch: fail with clear error message

**Deliverable**: Refinery correctly identifies protected branches and fails gracefully.

### Phase 2: PR Workflow (gt-0ae)
- Add `CreatePullRequest()` function using `gh pr create`
- Add `MergePullRequest()` function using `gh pr merge`
- Modify patrol formula to handle PR creation
- Integrate with existing MERGED notification flow

**Deliverable**: Refinery creates PRs for protected branches.

### Phase 3: Gate Integration (gt-0ug)
- Create `gh:pr` gates when PR is created
- Leverage existing Deacon `github-gate-check` step
- Dispatch merge completion when gate clears
- Handle PR review requirements

**Deliverable**: Full async workflow with gate-based PR monitoring.

## Appendix: Dimension Analyses

- [API & Interface Design](./api.md)
- [Data Model Design](./data.md)
- [Integration Analysis](./integration.md)
- [Security Analysis](./security.md)

## References

- [GitHub REST API: Branch Protection](https://docs.github.com/en/rest/branches/branch-protection)
- Current refinery code: `internal/refinery/engineer.go`
- Gate system: `internal/formula/formulas/mol-deacon-patrol.formula.toml`
