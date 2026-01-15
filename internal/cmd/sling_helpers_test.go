package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAttachPolecatWorkMolecule_PinsBeadBeforeAttach verifies that
// attachPolecatWorkMolecule pins the agent bead before calling AttachMolecule.
// This fixes the bug where AttachMolecule requires status=pinned but agent beads
// are created with status=open.
func TestAttachPolecatWorkMolecule_PinsBeadBeforeAttach(t *testing.T) {
	townRoot := t.TempDir()

	// Create minimal workspace structure
	rigName := "gastown"
	polecatName := "Toast"
	rigPath := filepath.Join(townRoot, rigName)

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rigPath, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir rig/.beads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create rigs.json for prefix lookup (config.GetRigPrefix needs this)
	rigsJSON := `{"rigs":{"gastown":{"path":"gastown","prefix":"gt"}}}`
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}

	// Create routes.jsonl for ResolveHookDir
	routes := `{"prefix":"gt","path":"gastown"}
{"prefix":"hq","path":"."}`
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Create stub bd script that logs all commands
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	bdPath := filepath.Join(binDir, "bd")

	// The bd stub:
	// - Logs all commands to BD_LOG
	// - Returns empty attachment for GetAttachment (show command)
	// - Returns success for cook
	// - Logs update with status=pinned
	// - Returns pinned status for show after update
	// - Returns success for attachment operations
	//
	// Note: beads package adds --no-daemon --allow-stale to all commands
	bdScript := `#!/bin/sh
echo "ARGS:$*" >> "${BD_LOG}"

# Find the actual command (skip --no-daemon, --allow-stale, etc.)
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) continue ;;
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  show)
    # Return agent bead with pinned status (simulates after pinning)
    # AttachMolecule requires status=pinned
    printf '[{"id":"gt-gastown-polecat-Toast","title":"polecat Toast","status":"pinned","description":""}]\n'
    ;;
  cook)
    exit 0
    ;;
  update)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Call attachPolecatWorkMolecule
	targetAgent := rigName + "/polecats/" + polecatName
	err := attachPolecatWorkMolecule(targetAgent, rigPath, townRoot)
	if err != nil {
		t.Fatalf("attachPolecatWorkMolecule failed: %v", err)
	}

	// Read the log to verify the sequence of commands
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd.log: %v", err)
	}
	logLines := strings.Split(string(logContent), "\n")

	// Verify that update --status=pinned was called
	foundPinUpdate := false
	foundCook := false
	pinUpdateIndex := -1
	cookIndex := -1

	for i, line := range logLines {
		if strings.Contains(line, "update") && strings.Contains(line, "--status=pinned") {
			foundPinUpdate = true
			pinUpdateIndex = i
		}
		if strings.Contains(line, "cook") && strings.Contains(line, "mol-polecat-work") {
			foundCook = true
			cookIndex = i
		}
	}

	if !foundCook {
		t.Error("expected bd cook mol-polecat-work to be called")
	}

	if !foundPinUpdate {
		t.Error("expected bd update --status=pinned to be called")
	}

	// Verify pinning happens AFTER cooking (cooking should happen first,
	// then pinning, then attachment)
	if foundCook && foundPinUpdate && pinUpdateIndex < cookIndex {
		t.Errorf("pinning should happen after cooking: cook at line %d, pin at line %d", cookIndex, pinUpdateIndex)
	}
}

// TestAttachPolecatWorkMolecule_SkipsIfAlreadyAttached verifies that
// attachPolecatWorkMolecule skips attachment if molecule is already attached.
func TestAttachPolecatWorkMolecule_SkipsIfAlreadyAttached(t *testing.T) {
	townRoot := t.TempDir()

	// Create minimal workspace structure
	rigName := "gastown"
	polecatName := "Toast"
	rigPath := filepath.Join(townRoot, rigName)

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rigPath, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir rig/.beads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create rigs.json
	rigsJSON := `{"rigs":{"gastown":{"path":"gastown","prefix":"gt"}}}`
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}

	// Create routes.jsonl
	routes := `{"prefix":"gt","path":"gastown"}
{"prefix":"hq","path":"."}`
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Create stub bd that returns an already-attached molecule
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	bdPath := filepath.Join(binDir, "bd")

	// This bd stub returns a bead with an attached molecule
	// Note: beads package adds --no-daemon --allow-stale to all commands
	bdScript := `#!/bin/sh
echo "ARGS:$*" >> "${BD_LOG}"

# Find the actual command (skip --no-daemon, --allow-stale, etc.)
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) continue ;;
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  show)
    # Return bead with already-attached molecule in description
    printf '[{"id":"gt-gastown-polecat-Toast","title":"polecat Toast","status":"pinned","description":"attached_molecule: mol-polecat-work\\nattached_at: 2024-01-01T00:00:00Z"}]\n'
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Call attachPolecatWorkMolecule
	targetAgent := rigName + "/polecats/" + polecatName
	err := attachPolecatWorkMolecule(targetAgent, rigPath, townRoot)
	if err != nil {
		t.Fatalf("attachPolecatWorkMolecule failed: %v", err)
	}

	// Read the log - should only have the initial show command, no cook/update
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd.log: %v", err)
	}
	logLines := string(logContent)

	// Should NOT have cook command since we skipped due to existing attachment
	if strings.Contains(logLines, "cook") {
		t.Error("expected to skip cooking when molecule already attached")
	}

	// Should NOT have update command
	if strings.Contains(logLines, "update") {
		t.Error("expected to skip update when molecule already attached")
	}
}

// TestAttachPolecatWorkMolecule_InvalidFormat tests error handling for invalid target format.
func TestAttachPolecatWorkMolecule_InvalidFormat(t *testing.T) {
	tests := []struct {
		name        string
		targetAgent string
		wantErr     bool
	}{
		{
			name:        "valid format",
			targetAgent: "gastown/polecats/Toast",
			wantErr:     false, // Would need full setup to pass
		},
		{
			name:        "missing polecats segment",
			targetAgent: "gastown/crew/max",
			wantErr:     true,
		},
		{
			name:        "too few segments",
			targetAgent: "gastown/Toast",
			wantErr:     true,
		},
		{
			name:        "single segment",
			targetAgent: "Toast",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For invalid format tests, we don't need a full setup
			// The function should fail early on format validation
			if tt.wantErr {
				err := attachPolecatWorkMolecule(tt.targetAgent, "/tmp", "/tmp")
				if err == nil {
					t.Error("expected error for invalid format")
				}
				if !strings.Contains(err.Error(), "invalid polecat agent format") {
					t.Errorf("expected 'invalid polecat agent format' error, got: %v", err)
				}
			}
		})
	}
}
