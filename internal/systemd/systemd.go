package systemd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ServiceState represents the high-level state of a systemd unit.
type ServiceState string

const (
	StateRunning  ServiceState = "running"
	StateStopped  ServiceState = "stopped"
	StateFailed   ServiceState = "failed"
	StateStarting ServiceState = "starting"
	StateStopping ServiceState = "stopping"
	StateUnknown  ServiceState = "unknown"
)

// ServiceStatus holds detailed information about a systemd unit.
type ServiceStatus struct {
	State       ServiceState
	SubState    string
	PID         int
	ActiveSince time.Time
	ExitCode    string
	Description string
}

// Manager controls a systemd unit via systemctl and journalctl.
type Manager struct {
	unit string
}

// NewManager creates a Manager for the given systemd unit name.
func NewManager(unit string) *Manager {
	return &Manager{unit: unit}
}

// Unit returns the systemd unit name.
func (m *Manager) Unit() string {
	return m.unit
}

// Start starts the systemd unit.
func (m *Manager) Start(ctx context.Context) error {
	return m.runSystemctl(ctx, "start")
}

// Stop stops the systemd unit.
func (m *Manager) Stop(ctx context.Context) error {
	return m.runSystemctl(ctx, "stop")
}

// Restart restarts the systemd unit.
func (m *Manager) Restart(ctx context.Context) error {
	return m.runSystemctl(ctx, "restart")
}

// Enable enables the systemd unit to start at boot.
func (m *Manager) Enable(ctx context.Context) error {
	return m.runSystemctl(ctx, "enable")
}

// DaemonReload reloads the systemd manager configuration.
func (m *Manager) DaemonReload(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "daemon-reload")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w\n%s", err, string(out))
	}
	return nil
}

// Status queries the unit and returns a parsed ServiceStatus.
func (m *Manager) Status(ctx context.Context) (*ServiceStatus, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "show", m.unit,
		"--property=ActiveState,SubState,MainPID,ActiveEnterTimestamp,ExecMainStatus,Description")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("systemctl show failed: %w", err)
	}

	props := parseProperties(out)

	status := &ServiceStatus{
		SubState:    props["SubState"],
		Description: props["Description"],
		ExitCode:    props["ExecMainStatus"],
	}

	// Map ActiveState + SubState to our ServiceState enum.
	status.State = mapState(props["ActiveState"], props["SubState"])

	if pidStr := props["MainPID"]; pidStr != "" {
		if pid, err := strconv.Atoi(pidStr); err == nil {
			status.PID = pid
		}
	}

	if ts := props["ActiveEnterTimestamp"]; ts != "" {
		status.ActiveSince = parseTimestamp(ts)
	}

	return status, nil
}

// IsActive returns true if the unit's ActiveState is "active".
func (m *Manager) IsActive(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", m.unit)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

// IsEnabled returns true if the unit is enabled.
func (m *Manager) IsEnabled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "systemctl", "is-enabled", m.unit)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "enabled"
}

// UnitExists returns true if the unit file is known to systemd.
func (m *Manager) UnitExists(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "systemctl", "list-unit-files", m.unit)
	if err := cmd.Run(); err != nil {
		return false
	}
	// list-unit-files exits 0 even when no matches. Check output for the unit name.
	out, err := exec.CommandContext(ctx, "systemctl", "list-unit-files", m.unit).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), m.unit)
}

// Logs returns an unstarted *exec.Cmd for journalctl that follows log output.
// The caller is responsible for starting the command and reading from its
// Stdout pipe. Stderr is also piped.
func (m *Manager) Logs(ctx context.Context, lines int, since string, follow bool) (*exec.Cmd, error) {
	args := m.buildJournalctlArgs(lines, since, follow)
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	// Leave Stdout and Stderr as nil so the caller can call cmd.StdoutPipe()
	// or set them before starting the command.
	return cmd, nil
}

// LogLines runs journalctl (non-follow) and returns the output as a slice of strings.
func (m *Manager) LogLines(ctx context.Context, lines int, since string) ([]string, error) {
	args := m.buildJournalctlArgs(lines, since, false)
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("journalctl failed: %w", err)
	}

	var result []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			result = append(result, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning journalctl output: %w", err)
	}

	return result, nil
}

// runSystemctl runs a systemctl verb against the unit.
func (m *Manager) runSystemctl(ctx context.Context, verb string) error {
	cmd := exec.CommandContext(ctx, "systemctl", verb, m.unit)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %s %s failed: %w\n%s", verb, m.unit, err, string(out))
	}
	return nil
}

// buildJournalctlArgs constructs the argument list for journalctl.
func (m *Manager) buildJournalctlArgs(lines int, since string, follow bool) []string {
	args := []string{"-u", m.unit, "--no-pager"}

	if follow {
		args = append(args, "--follow", "--output", "cat")
	} else {
		args = append(args, "--output", "short-iso")
	}

	if lines > 0 {
		args = append(args, "-n", strconv.Itoa(lines))
	}

	if since != "" {
		args = append(args, "--since", since)
	}

	return args
}

// parseProperties parses "Key=Value" lines from systemctl show output.
func parseProperties(data []byte) map[string]string {
	props := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.IndexByte(line, '='); idx >= 0 {
			key := line[:idx]
			value := line[idx+1:]
			props[key] = value
		}
	}
	return props
}

// mapState translates systemd ActiveState/SubState into our ServiceState.
func mapState(active, sub string) ServiceState {
	switch active {
	case "active":
		if sub == "running" {
			return StateRunning
		}
		return StateRunning
	case "inactive":
		return StateStopped
	case "failed":
		return StateFailed
	case "activating":
		return StateStarting
	case "deactivating":
		return StateStopping
	default:
		return StateUnknown
	}
}

// parseTimestamp parses the timestamp format returned by systemctl show, e.g.
// "Mon 2024-01-15 10:30:45 UTC" or "Day YYYY-MM-DD HH:MM:SS TZ".
func parseTimestamp(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" || s == "n/a" {
		return time.Time{}
	}

	// systemctl show outputs timestamps like "Mon 2024-01-15 10:30:45 UTC"
	// Try the most common format first.
	layouts := []string{
		"Mon 2006-01-02 15:04:05 MST",
		"Mon 2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05 -0700",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}

	return time.Time{}
}
