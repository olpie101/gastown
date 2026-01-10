# API & Interface Design

## Summary

The refinery needs to detect when a target branch has protection rules before attempting to push. This requires integrating with GitHub's REST API for branch protection and potentially using PR-based merge workflows instead of direct pushes when protection is detected.

The recommended approach uses the `gh` CLI (already available in the codebase) to query branch protection status, maintaining consistency with existing GitHub integrations.

## Analysis

### Key Considerations

- **Existing pattern**: The codebase uses `gh` CLI for GitHub operations (`gh api user`, `gh pr list`)
- **Authentication**: `gh` CLI handles auth automatically via existing user session or tokens
- **Rate limits**: GitHub API has rate limits (5000/hour for authenticated requests)
- **Permissions**: Querying branch protection requires `administration:read` permission
- **Protected branch behaviors**: Required reviews, status checks, conversation resolution, etc.

### Options Explored

#### Option 1: Use `gh` CLI (Recommended)
- **Description**: Use `gh api repos/{owner}/{repo}/branches/{branch}/protection` command
- **Pros**:
  - Consistent with existing codebase patterns
  - Handles authentication automatically
  - No new dependencies
  - Portable across environments
- **Cons**:
  - Subprocess overhead
  - Parsing JSON output required
- **Effort**: Low

#### Option 2: Direct HTTP Client
- **Description**: Use Go's `net/http` with GitHub REST API directly
- **Pros**:
  - More control over requests
  - No subprocess overhead
  - Better error handling
- **Cons**:
  - Need to handle authentication tokens
  - New code paths to maintain
  - Token management complexity
- **Effort**: Medium

#### Option 3: GitHub Go SDK (google/go-github)
- **Description**: Use the official Go client library
- **Pros**:
  - Type-safe API
  - Built-in pagination and rate limiting
  - Well-maintained
- **Cons**:
  - New dependency
  - Larger binary size
  - Learning curve
- **Effort**: Medium-High

### Recommendation

**Use Option 1 (gh CLI)** for consistency and minimal effort. The existing codebase already uses this pattern in:
- `internal/config/overseer.go:162` - `gh api user`
- `internal/web/fetcher.go:483` - `gh pr list`

Implementation via helper function:
```go
// CheckBranchProtection returns protection status for a branch
func (g *GitHub) CheckBranchProtection(owner, repo, branch string) (*BranchProtection, error) {
    cmd := exec.Command("gh", "api",
        fmt.Sprintf("repos/%s/%s/branches/%s/protection", owner, repo, branch),
        "--jq", ".required_pull_request_reviews != null or .required_status_checks != null")
    // ...
}
```

## Constraints Identified

1. **404 on unprotected branches**: GitHub returns 404 when a branch has no protection, not an empty object
2. **Permission requirement**: Tokens need `administration:read` scope to query protection
3. **API versioning**: Must set `X-GitHub-Api-Version: 2022-11-28` header
4. **Rate limiting**: Should cache protection status to avoid repeated queries

## Open Questions

1. Should protection status be cached per-session or persisted to disk?
2. What's the fallback behavior if `gh` CLI is not installed?
3. Should we check protection once per MR or on every push attempt?

## Integration Points

- **Data dimension**: Protection status needs to be stored/cached somewhere
- **Integration dimension**: Fits into refinery patrol before `merge-push` step
- **Security dimension**: Token permissions must be documented
