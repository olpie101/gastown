# Security Analysis

## Summary

Protected branch handling introduces new trust boundaries with the GitHub API. The refinery needs read access to branch protection rules and write access to create PRs. Token scope management is critical to prevent privilege escalation while enabling the required functionality.

The key security concerns are token permission scope, rate limiting abuse, and ensuring the system doesn't bypass protection rules it's meant to respect.

## Analysis

### Key Considerations

- **Current authentication**: `gh` CLI uses user's GitHub auth (oauth token or SSH)
- **New permissions needed**: `administration:read` for protection queries
- **Existing token usage**: Codebase uses `gh api user` which works with basic scopes
- **Trust model**: Refinery runs in user's session with user's credentials
- **Attack surface**: New API endpoints exposed

### Trust Boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│                        User's Machine                           │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────────┐   │
│  │  Refinery   │────▶│   gh CLI    │────▶│ GitHub OAuth    │   │
│  │  (Claude)   │     │             │     │ Token (~/.config│   │
│  └─────────────┘     └─────────────┘     │ /gh/hosts.yaml) │   │
│                                          └────────┬────────┘   │
└────────────────────────────────────────────────────│───────────┘
                                                     │
                                                     ▼
                                          ┌─────────────────────┐
                                          │    GitHub API       │
                                          │  (repos/branches/   │
                                          │   protection)       │
                                          └─────────────────────┘
```

**Trust levels**:
1. **Refinery (Claude)**: Trusted agent running as user
2. **gh CLI**: Trusted tool, manages token securely
3. **GitHub Token**: User-controlled, scoped permissions
4. **GitHub API**: External service, rate-limited

### Threat Model

| Threat | Impact | Likelihood | Mitigation |
|--------|--------|------------|------------|
| Token over-scoping | High | Medium | Document minimum required scopes |
| Rate limit exhaustion | Low | Low | Cache protection status |
| Protection bypass | High | Low | Never skip protection checks programmatically |
| Token exposure in logs | High | Low | Use `--silent` flags, don't log tokens |
| Privilege escalation via PR | Medium | Low | PRs still require reviews if configured |

### Options Explored

#### Option 1: Use Existing gh Auth (Recommended)
- **Description**: Rely on user's existing `gh` CLI authentication
- **Pros**:
  - No new credential management
  - User controls permissions
  - Follows principle of least privilege by default
  - Token already trusted for `gh api user`
- **Cons**:
  - May need to prompt user to add `administration:read` scope
  - User could have over-scoped token
- **Effort**: Low

#### Option 2: Dedicated Service Token
- **Description**: Create a dedicated GitHub App or PAT for Gas Town
- **Pros**:
  - Precise scope control
  - Audit trail for Gas Town actions
  - Doesn't use user's personal credentials
- **Cons**:
  - Token management complexity
  - Installation friction for users
  - Need secure storage
- **Effort**: High

#### Option 3: Read-Only Mode Fallback
- **Description**: If protection check fails (403), assume protected and use safe path
- **Pros**:
  - Works even without administration scope
  - Fails safe (assumes protected)
- **Cons**:
  - False positives for permission errors vs actual protection
  - May create unnecessary PRs
- **Effort**: Low

### Recommendation

**Use Option 1 (existing gh auth)** with Option 3 as fallback:

1. Attempt protection check with current token
2. If 403 (permission denied): Warn user, assume protected, proceed with PR flow
3. If 404 (not protected): Use direct push
4. If 200 (protected): Use PR flow

### Permission Requirements

**Minimum required GitHub token scopes**:

| Scope | Purpose | Required For |
|-------|---------|--------------|
| `repo` | Basic repo access | Existing functionality |
| `read:org` | Org membership | Team-based restrictions |
| `administration:read` | Branch protection query | Protection detection |
| `pull_request:write` | Create PRs | PR-based merge flow |

**To check current scopes**:
```bash
gh auth status
# Or:
curl -sS -H "Authorization: token $(gh auth token)" \
  https://api.github.com/rate_limit -I | grep x-oauth-scopes
```

**To add missing scopes**:
```bash
gh auth refresh -s admin:read
```

### Security Controls

1. **Logging**: Never log tokens or auth headers
   ```go
   // GOOD
   log.Printf("Checking protection for %s/%s:%s", owner, repo, branch)

   // BAD - never do this
   log.Printf("Using token %s", token)
   ```

2. **Error messages**: Don't expose token details in errors
   ```go
   // GOOD
   return fmt.Errorf("protection check failed: insufficient permissions")

   // BAD
   return fmt.Errorf("protection check failed: token xyz lacks admin scope")
   ```

3. **Cache security**: Protection cache contains only branch names and rules, no secrets

4. **Command injection**: Sanitize branch names before use in commands
   ```go
   // GOOD - validate input
   if !isValidBranchName(branch) {
       return fmt.Errorf("invalid branch name")
   }

   // Use exec.Command with separate args (no shell interpolation)
   cmd := exec.Command("gh", "api", path)
   ```

### Attack Surface Changes

| New Surface | Risk | Mitigation |
|-------------|------|------------|
| Protection API calls | Rate limiting | Cache with TTL |
| PR creation | Spam/abuse | Only create for actual merges |
| PR merge via API | Bypass checks | API respects all protection rules |

## Constraints Identified

1. **Token scope**: Users may need to re-authenticate to add scopes
2. **Org settings**: Some orgs may restrict API access
3. **Audit logs**: GitHub Enterprise logs all API access
4. **Rate limits**: 5000 requests/hour for authenticated users

## Open Questions

1. Should we detect insufficient permissions at startup and warn early?
2. How to handle SSO-protected orgs that require re-auth?
3. Should we support GitHub App authentication for org-wide deployment?

## Integration Points

- **API dimension**: Token scopes affect which endpoints work
- **Data dimension**: No secrets stored in protection cache
- **Integration dimension**: Error handling for permission failures
