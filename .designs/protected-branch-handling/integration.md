# Integration Analysis

## Summary

Protected branch detection integrates into the refinery patrol flow at the `merge-push` step. When a protected branch is detected, the refinery must switch from direct push to a PR-based workflow using GitHub's API. This requires adding a protection check before the push and potentially a new step for PR-based merging.

The recommended approach adds detection before push with a fallback to PR workflow when direct push is rejected.

## Analysis

### Key Considerations

- **Detection timing**: Check protection before attempting push to avoid wasted effort
- **Existing flow**: `merge-push` step currently does direct `git push origin main`
- **Gate system**: Existing `gh:pr` gates in Deacon can handle PR status checks
- **Error handling**: Current flow treats all push failures the same (`FailurePushFail`)
- **Backwards compatibility**: Unprotected branches should work unchanged

### Current Merge Flow (mol-refinery-patrol.formula.toml)

```
inbox-check → queue-scan → process-branch → run-tests → handle-failures → merge-push → loop-check
```

The `merge-push` step currently:
1. `git checkout main`
2. `git merge --ff-only temp`
3. `git push origin main` ← **Failure point for protected branches**
4. Send MERGED notification
5. Close MR bead
6. Cleanup branches

### Options Explored

#### Option 1: Pre-Push Detection (Recommended)
- **Description**: Check branch protection before attempting push; route to PR workflow if protected
- **Pros**:
  - Avoids failed push attempts
  - Clean error flow
  - User sees correct path from start
- **Cons**:
  - Extra API call per merge
  - Cache needed to avoid repeated queries
- **Effort**: Medium

#### Option 2: Catch-and-Retry on Push Failure
- **Description**: Attempt push first; on rejection, detect protection and switch to PR
- **Pros**:
  - No upfront API call for unprotected branches
  - Works without protection cache
- **Cons**:
  - Noisy error logs for protected branches
  - Harder to distinguish protection failure from other push failures
- **Effort**: Low-Medium

#### Option 3: Configuration-Driven
- **Description**: User specifies which branches are protected in config
- **Pros**:
  - No API calls needed
  - Predictable behavior
- **Cons**:
  - Config drift from reality
  - Manual maintenance burden
  - Doesn't catch new protection rules
- **Effort**: Low

### Recommendation

**Use Option 1 (Pre-Push Detection)** with caching. Add a new step or modify `merge-push`:

**Modified flow**:
```
... → run-tests → handle-failures → [check-protection] → merge-push → ...
                                            ↓
                                     (if protected)
                                            ↓
                                    merge-via-pr → wait-for-pr → ...
```

### Integration Points

#### 1. Protection Check (new logic in merge-push or separate step)

```toml
# Insert before push in merge-push step, or as new step:
[[steps]]
id = "check-protection"
title = "Check target branch protection"
needs = ["handle-failures"]
description = """
Check if target branch has protection rules.

```bash
# Check protection status
gh api repos/{owner}/{repo}/branches/{target}/protection --silent 2>/dev/null
# Exit code 0 = protected, Exit code 1 (404) = not protected
```

If protected:
- Proceed to merge-via-pr step
- Skip direct push in merge-push

If not protected:
- Proceed to merge-push as normal
"""
```

#### 2. PR-Based Merge (new step)

```toml
[[steps]]
id = "merge-via-pr"
title = "Create PR for protected branch"
needs = ["check-protection"]  # only when protected
description = """
Create and merge PR for protected branch.

**Step 1: Create PR**
```bash
gh pr create --base main --head temp \
  --title "Merge <issue-id>: <title>" \
  --body "Automated merge from refinery.
Issue: <issue-id>
Branch: <polecat-branch>
MR: <mr-bead-id>"
```

**Step 2: Wait for required checks**
Create a gate for the PR:
```bash
bd gate create --await gh:pr:<pr-number> --waiter <self-mail>
```

Park on the gate and move to next MR in queue.
Gate system handles check polling.

**Step 3: Merge when checks pass**
When gate clears (via Deacon dispatch):
```bash
gh pr merge <pr-number> --merge --delete-branch
```

Proceed to send MERGED notification and cleanup.
"""
```

#### 3. Deacon Gate Handling (existing system)

The Deacon's `github-gate-check` step already handles `gh:pr` gates:
- Polls PR status via `bd gate check --type=gh`
- Closes gates when PR is merged
- Dispatches waiting molecules

This existing infrastructure handles the async PR wait.

### Code Changes Required

| File | Change |
|------|--------|
| `internal/refinery/engineer.go` | Add `checkBranchProtection()` before `doMerge()` |
| `internal/refinery/types.go` | Add `FailureProtectedBranch` failure type |
| `internal/refinery/github.go` (new) | GitHub API helpers for protection check, PR creation |
| `mol-refinery-patrol.formula.toml` | Document protection check in merge-push or add new step |
| `internal/cmd/mq.go` | Consider adding `--protected-branch-mode` flag |

### Existing Components Leveraged

1. **Gate system**: `bd gate create --await gh:pr:NNN` for async PR wait
2. **Deacon patrol**: `github-gate-check` step handles `gh:pr` gates
3. **gh CLI**: Already used for `gh api`, `gh pr list`
4. **Mail system**: MERGED notification flow unchanged

## Constraints Identified

1. **Async complexity**: PR workflow is async; refinery can't block waiting
2. **Gate system dependency**: Relies on Deacon running to poll PR status
3. **PR permissions**: Need write access to create PRs
4. **Merge method**: Some repos require squash merge vs merge commit

## Open Questions

1. Should protection check be a separate formula step or inline in merge-push?
2. How to handle required reviews (need human or bot approval)?
3. Should we support "required reviewers" by auto-requesting reviews?
4. What merge method to use (merge, squash, rebase)?

## Migration Path

1. **Phase 1**: Add protection detection, fail gracefully with clear error
2. **Phase 2**: Implement PR creation, manual merge
3. **Phase 3**: Gate-based async PR merge flow
