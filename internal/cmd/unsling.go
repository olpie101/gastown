package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/style"
)

var unslingCmd = &cobra.Command{
	Use:     "unsling [bead-id] [target]",
	Aliases: []string{"unhook"},
	GroupID: GroupWork,
	Short:   "Remove work from an agent's hook",
	Long: `Remove work from an agent's hook (the inverse of sling/hook).

With no arguments, clears your own hook. With a bead ID, only unslings
if that specific bead is currently hooked. With a target, operates on
another agent's hook.

Examples:
  gt unsling                        # Clear my hook (whatever's there)
  gt unsling gt-abc                 # Only unsling if gt-abc is hooked
  gt unsling greenplace/joe            # Clear joe's hook
  gt unsling gt-abc greenplace/joe     # Unsling gt-abc from joe

The bead's status changes from 'pinned' back to 'open'.

Related commands:
  gt sling <bead>    # Hook + start (inverse of unsling)
  gt hook <bead>     # Hook without starting
  gt mol status      # See what's on your hook`,
	Args: cobra.MaximumNArgs(2),
	RunE: runUnsling,
}

var (
	unslingDryRun bool
	unslingForce  bool
)

func init() {
	unslingCmd.Flags().BoolVarP(&unslingDryRun, "dry-run", "n", false, "Show what would be done")
	unslingCmd.Flags().BoolVarP(&unslingForce, "force", "f", false, "Unsling even if work is incomplete")
	rootCmd.AddCommand(unslingCmd)
}

func runUnsling(cmd *cobra.Command, args []string) error {
	var targetBeadID string
	var targetAgent string

	// Parse args: [bead-id] [target]
	switch len(args) {
	case 0:
		// No args - unsling self, whatever is hooked
	case 1:
		// Could be bead ID or target agent
		// If it contains "/" or is a known role, treat as target
		if isAgentTarget(args[0]) {
			targetAgent = args[0]
		} else {
			targetBeadID = args[0]
		}
	case 2:
		targetBeadID = args[0]
		targetAgent = args[1]
	}

	// Resolve target agent (default: self)
	var agentID string
	var err error
	if targetAgent != "" {
		// Skip pane lookup - unsling only needs agent ID, not tmux session
		agentID, _, _, err = resolveTargetAgent(targetAgent, true)
		if err != nil {
			return fmt.Errorf("resolving target agent: %w", err)
		}
	} else {
		agentID, _, _, err = resolveSelfTarget()
		if err != nil {
			return fmt.Errorf("detecting agent identity: %w", err)
		}
	}

	// Find beads directory
	workDir, err := findLocalBeadsDir()
	if err != nil {
		return fmt.Errorf("not in a beads workspace: %w", err)
	}

	b := beads.New(workDir)

	// Find pinned bead for this agent
	pinnedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusPinned,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		return fmt.Errorf("checking pinned beads: %w", err)
	}

	if len(pinnedBeads) == 0 {
		if targetAgent != "" {
			fmt.Printf("%s No work hooked for %s\n", style.Dim.Render("ℹ"), agentID)
		} else {
			fmt.Printf("%s Nothing on your hook\n", style.Dim.Render("ℹ"))
		}
		return nil
	}

	pinned := pinnedBeads[0]

	// If specific bead requested, verify it matches
	if targetBeadID != "" && pinned.ID != targetBeadID {
		return fmt.Errorf("bead %s is not hooked (current hook: %s)", targetBeadID, pinned.ID)
	}

	// Check if work is complete (warn if not, unless --force)
	isComplete, _ := checkPinnedBeadComplete(b, pinned)
	if !isComplete && !unslingForce {
		return fmt.Errorf("hooked work %s is incomplete (%s)\n  Use --force to unsling anyway",
			pinned.ID, pinned.Title)
	}

	if targetAgent != "" {
		fmt.Printf("%s Unslinging %s from %s...\n", style.Bold.Render("🪝"), pinned.ID, agentID)
	} else {
		fmt.Printf("%s Unslinging %s...\n", style.Bold.Render("🪝"), pinned.ID)
	}

	if unslingDryRun {
		fmt.Printf("Would run: bd update %s --status=open\n", pinned.ID)
		return nil
	}

	// Unpin by setting status back to open
	status := "open"
	if err := b.Update(pinned.ID, beads.UpdateOptions{Status: &status}); err != nil {
		return fmt.Errorf("unpinning bead %s: %w", pinned.ID, err)
	}

	// Log unhook event
	_ = events.LogFeed(events.TypeUnhook, agentID, events.UnhookPayload(pinned.ID))

	fmt.Printf("%s Work removed from hook\n", style.Bold.Render("✓"))
	fmt.Printf("  Bead %s is now status=open\n", pinned.ID)

	return nil
}

// isAgentTarget checks if a string looks like an agent target rather than a bead ID.
// Agent targets contain "/" or are known role names.
func isAgentTarget(s string) bool {
	// Contains "/" means it's a path like "greenplace/joe"
	for _, c := range s {
		if c == '/' {
			return true
		}
	}

	// Known role names
	switch s {
	case "mayor", "deacon", "witness", "refinery", "crew":
		return true
	}

	return false
}
