+++
name = "poll-pr-gates"
description = "Poll GitHub for PR merge status on open gh:pr gates"
version = 1

[gate]
type = "cooldown"
duration = "5m"

[tracking]
labels = ["plugin:poll-pr-gates", "category:gates"]
digest = true

[execution]
timeout = "2m"
notify_on_failure = true
severity = "medium"
+++

# Poll PR Gates

Checks all open gh:pr gates and closes any where the PR has merged.

## Overview

This plugin runs periodically during Deacon patrol cycles to:
1. Find all open gates with `gh:pr:` await type
2. Query GitHub for each PR's current state
3. Close gates where the PR has merged
4. Send wake notifications to trigger post-merge cleanup

## Detection

List open gh:pr gates:

```bash
GATES=$(bd gate list --json 2>/dev/null | jq -r '.[]? | select(.await_type | startswith("gh:pr:")) | .id')
if [ -z "$GATES" ]; then
  echo "No open gh:pr gates"
  exit 0
fi
echo "Found $(echo "$GATES" | wc -l) open gh:pr gate(s)"
```

## Action

For each gate, check PR status and close if merged:

```bash
CLOSED=0
CHECKED=0

for GATE_ID in $GATES; do
  CHECKED=$((CHECKED + 1))

  # Get gate details
  GATE_JSON=$(bd gate show "$GATE_ID" --json 2>/dev/null)
  AWAIT_TYPE=$(echo "$GATE_JSON" | jq -r '.await_type')

  # Extract PR number from await_type (gh:pr:123 -> 123)
  PR_NUMBER=$(echo "$AWAIT_TYPE" | cut -d':' -f3)

  if [ -z "$PR_NUMBER" ]; then
    echo "Warning: Could not extract PR number from $AWAIT_TYPE"
    continue
  fi

  # Query GitHub for PR state
  PR_STATE=$(gh pr view "$PR_NUMBER" --json state -q '.state' 2>/dev/null)

  case "$PR_STATE" in
    MERGED)
      echo "PR #$PR_NUMBER merged - closing gate $GATE_ID"
      bd gate close "$GATE_ID" --reason "PR #$PR_NUMBER merged"
      gt gate wake "$GATE_ID"
      CLOSED=$((CLOSED + 1))
      ;;
    CLOSED)
      echo "PR #$PR_NUMBER closed without merge - closing gate $GATE_ID"
      bd gate close "$GATE_ID" --reason "PR #$PR_NUMBER closed without merge"
      gt gate wake "$GATE_ID"
      CLOSED=$((CLOSED + 1))
      ;;
    OPEN)
      echo "PR #$PR_NUMBER still open - gate $GATE_ID remains open"
      ;;
    *)
      echo "Warning: Unknown PR state '$PR_STATE' for PR #$PR_NUMBER"
      ;;
  esac
done

echo "Checked $CHECKED gates, closed $CLOSED"
```

## Result Recording

The plugin system automatically records runs as wisps with the configured labels.

## Rate Limiting

- Cooldown gate ensures max one run per 5 minutes
- Processing limited to available gates (typically few)
- GitHub API rate limits apply (5000/hour authenticated)

## Dependencies

- `bd gate list` - List open gates
- `bd gate show` - Get gate details
- `bd gate close` - Close gate
- `gt gate wake` - Notify waiters
- `gh pr view` - Query PR status from GitHub
