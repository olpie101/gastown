package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// Peek command flags
var peekLines int

func init() {
	rootCmd.AddCommand(peekCmd)
	peekCmd.Flags().IntVarP(&peekLines, "lines", "n", 100, "Number of lines to capture")
}

var peekCmd = &cobra.Command{
	Use:     "peek <rig/polecat> [count]",
	GroupID: GroupComm,
	Short:   "View recent output from a polecat session",
	Long: `Capture and display recent terminal output from a polecat session.

This is the ergonomic alias for 'gt session capture'. Use it to check
what an agent is currently doing or has recently output.

The nudge/peek pair provides the canonical interface for agent sessions:
  gt nudge - send messages TO a session (reliable delivery)
  gt peek  - read output FROM a session (capture-pane wrapper)

Examples:
  gt peek greenplace/furiosa         # Last 100 lines (default)
  gt peek greenplace/furiosa 50      # Last 50 lines
  gt peek greenplace/furiosa -n 200  # Last 200 lines`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runPeek,
}

func runPeek(cmd *cobra.Command, args []string) error {
	address := args[0]

	// Handle optional positional count argument
	lines := peekLines
	if len(args) > 1 {
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid line count: %s", args[1])
		}
		lines = n
	}

	rigName, polecatName, err := parseAddress(address)
	if err != nil {
		return err
	}

	mgr, _, err := getSessionManager(rigName)
	if err != nil {
		return err
	}

	output, err := mgr.Capture(polecatName, lines)
	if err != nil {
		return fmt.Errorf("capturing output: %w", err)
	}

	fmt.Print(output)
	return nil
}
