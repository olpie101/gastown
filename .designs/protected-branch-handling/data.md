# Data Model Design

## Summary

The refinery needs to track branch protection status to determine the appropriate merge strategy. This involves adding new data structures for protection state, extending existing types, and implementing a caching strategy to avoid repeated API calls.

The recommended approach adds minimal new state: a protection cache keyed by branch name, and a new failure type for protected branch rejections.

## Analysis

### Key Considerations

- **Existing structures**: `MergeRequest` already has `TargetBranch` field
- **Failure handling**: `FailureType` enum exists with push failures
- **Configuration**: `MergeConfig` handles merge behavior settings
- **Cache location**: In-memory vs disk persistence
- **TTL strategy**: Protection rules rarely change, so caching is safe

### Current Data Model (Relevant Parts)

```go
// types.go - MergeRequest
type MergeRequest struct {
    TargetBranch string         // Where this should merge
    Status       MRStatus       // Current status
    CloseReason  CloseReason    // Why closed (if closed)
}

// types.go - FailureType
const (
    FailurePushFail FailureType = "push_fail"  // Generic push failure
    // ... others
)
```

### Options Explored

#### Option 1: Extend MergeRequest with Protection Field
- **Description**: Add `ProtectedBranch bool` and `RequiresPR bool` to MergeRequest
- **Pros**:
  - Protection status travels with the MR
  - Clear ownership of data
- **Cons**:
  - Couples protection check to MR lifecycle
  - Redundant if same target branch is used repeatedly
- **Effort**: Low

#### Option 2: Protection Status Cache (Recommended)
- **Description**: Separate in-memory cache of branch protection status
- **Pros**:
  - Single query per branch per session
  - Reusable across MRs
  - Decoupled from MR lifecycle
- **Cons**:
  - New data structure to maintain
  - Cache invalidation concerns
- **Effort**: Low

#### Option 3: Persistent Protection Database
- **Description**: Store protection status in a JSON/SQLite file
- **Pros**:
  - Survives refinery restarts
  - Reduces API calls across sessions
- **Cons**:
  - Over-engineering for this use case
  - Stale data risk
  - Additional file I/O
- **Effort**: Medium-High

### Recommendation

**Use Option 2 (Protection Status Cache)** with these additions:

```go
// New types in types.go

// BranchProtection represents cached protection status for a branch.
type BranchProtection struct {
    Branch              string    `json:"branch"`
    IsProtected         bool      `json:"is_protected"`
    RequiresPullRequest bool      `json:"requires_pr"`
    RequiredReviewers   int       `json:"required_reviewers"`
    RequiredChecks      []string  `json:"required_checks"`
    CheckedAt           time.Time `json:"checked_at"`
}

// Add new failure type
const (
    // FailureProtectedBranch indicates push was rejected due to branch protection.
    FailureProtectedBranch FailureType = "protected_branch"
)

// FailureLabel extension
func (f FailureType) FailureLabel() string {
    switch f {
    case FailureProtectedBranch:
        return "needs-pr"
    // ... existing cases
    }
}
```

**Cache structure**:
```go
// In engineer.go or new protection.go
type ProtectionCache struct {
    mu     sync.RWMutex
    cache  map[string]*BranchProtection  // key: "owner/repo:branch"
    ttl    time.Duration                 // default: 1 hour
}
```

## Data Lifecycle

1. **Creation**: Cache entry created on first protection check for a branch
2. **Read**: Checked before each merge attempt
3. **Update**: TTL-based expiry, forced refresh on 403/auth errors
4. **Deletion**: Entry expires after TTL or session ends

## Constraints Identified

1. **No persistent storage**: Cache lost on refinery restart (acceptable for MVP)
2. **TTL trade-off**: Too short = more API calls, too long = stale data
3. **GitHub API 404**: Unprotected branches return 404, not empty object

## Open Questions

1. Should we store protection status in the MR for debugging/logging?
2. What TTL is appropriate? (Proposed: 1 hour, configurable)
3. Should the cache key include remote URL for multi-remote scenarios?

## Integration Points

- **API dimension**: Cache populated via `gh api` calls
- **Integration dimension**: Cache consulted before `merge-push` step
- **Security dimension**: No sensitive data stored (just branch names and rules)
