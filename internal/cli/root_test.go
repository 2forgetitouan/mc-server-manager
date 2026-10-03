package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRootCommandExists(t *testing.T) {
	cmd := newRootCmd()
	if cmd == nil {
		t.Fatal("newRootCmd returned nil")
	}
	if cmd.Use != "mc" {
		t.Errorf("Use = %q, want mc", cmd.Use)
	}
}

func TestRootCommandHasSubcommands(t *testing.T) {
	cmd := newRootCmd()
	subcommands := cmd.Commands()

	if len(subcommands) == 0 {
		t.Fatal("root command has no subcommands")
	}

	expectedCommands := []string{
		"start", "stop", "restart", "status", "logs",
		"console", "update", "doctor", "config", "version",
	}

	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	for _, expected := range expectedCommands {
		if !subNames[expected] {
			t.Errorf("missing subcommand %q", expected)
		}
	}
}

func TestVersionCommandOutput(t *testing.T) {
	SetVersion("1.2.3-test")
	defer SetVersion("")

	// The version command writes to os.Stdout via fmt.Printf, so we
	// capture stdout to verify output.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newRootCmd()
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "1.2.3-test") {
		t.Errorf("version output missing version string: %q", output)
	}
	if !strings.Contains(output, "go:") {
		t.Errorf("version output missing go version: %q", output)
	}
	if !strings.Contains(output, "os:") {
		t.Errorf("version output missing os info: %q", output)
	}
}

func TestHelpOutput(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Minecraft Server Manager") {
		t.Errorf("help output missing description: %q", output)
	}
}

func TestColorFlag(t *testing.T) {
	cmd := newRootCmd()
	flag := cmd.PersistentFlags().Lookup("color")
	if flag == nil {
		t.Fatal("--color flag not found")
	}
	if flag.DefValue != "auto" {
		t.Errorf("--color default = %q, want auto", flag.DefValue)
	}
}

func TestExecute(t *testing.T) {
	// Execute with version command should succeed.
	SetVersion("test")
	defer SetVersion("")

	// We can't easily test Execute() since it creates its own command,
	// but we can verify it doesn't panic with the version subcommand.
	cmd := newRootCmd()
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute with version failed: %v", err)
	}
}

func TestSubcommandShortDescriptions(t *testing.T) {
	cmd := newRootCmd()

	tests := []struct {
		name  string
		short string
	}{
		{"version", "Show manager version"},
	}

	for _, tt := range tests {
		sub, _, err := cmd.Find([]string{tt.name})
		if err != nil {
			t.Errorf("Find(%q) failed: %v", tt.name, err)
			continue
		}
		if sub.Short != tt.short {
			t.Errorf("%s.Short = %q, want %q", tt.name, sub.Short, tt.short)
		}
	}
}
