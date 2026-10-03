package systemd

import (
	"testing"
	"time"
)

func TestParseProperties(t *testing.T) {
	input := []byte(`ActiveState=active
SubState=running
MainPID=12345
ActiveEnterTimestamp=Mon 2024-01-15 10:30:45 UTC
ExecMainStatus=0
Description=Minecraft Server
`)
	props := parseProperties(input)

	tests := []struct {
		key, want string
	}{
		{"ActiveState", "active"},
		{"SubState", "running"},
		{"MainPID", "12345"},
		{"ActiveEnterTimestamp", "Mon 2024-01-15 10:30:45 UTC"},
		{"ExecMainStatus", "0"},
		{"Description", "Minecraft Server"},
	}

	for _, tt := range tests {
		got := props[tt.key]
		if got != tt.want {
			t.Errorf("props[%q] = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestParseProperties_EmptyInput(t *testing.T) {
	props := parseProperties([]byte{})
	if len(props) != 0 {
		t.Errorf("expected empty map, got %v", props)
	}
}

func TestParseProperties_NoEquals(t *testing.T) {
	input := []byte("just a line with no equals\nanother line\n")
	props := parseProperties(input)
	if len(props) != 0 {
		t.Errorf("expected empty map for lines without =, got %v", props)
	}
}

func TestParseProperties_EmptyValue(t *testing.T) {
	input := []byte("MainPID=\n")
	props := parseProperties(input)
	if got := props["MainPID"]; got != "" {
		t.Errorf("props[MainPID] = %q, want empty", got)
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			name:  "standard format with day name",
			input: "Mon 2024-01-15 10:30:45 UTC",
			want:  time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		},
		{
			name:  "format without day name",
			input: "2024-01-15 10:30:45 UTC",
			want:  time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		},
		{
			name:  "empty string",
			input: "",
			want:  time.Time{},
		},
		{
			name:  "n/a",
			input: "n/a",
			want:  time.Time{},
		},
		{
			name:  "unparseable",
			input: "not a timestamp at all",
			want:  time.Time{},
		},
		{
			name:  "whitespace only",
			input: "   ",
			want:  time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimestamp(tt.input)
			if !got.Equal(tt.want) {
				t.Errorf("parseTimestamp(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapState(t *testing.T) {
	tests := []struct {
		active, sub string
		want        ServiceState
	}{
		{"active", "running", StateRunning},
		{"active", "exited", StateRunning},
		{"inactive", "dead", StateStopped},
		{"failed", "failed", StateFailed},
		{"activating", "start", StateStarting},
		{"deactivating", "stop", StateStopping},
		{"", "", StateUnknown},
		{"maintenance", "unknown", StateUnknown},
	}

	for _, tt := range tests {
		name := tt.active + "/" + tt.sub
		t.Run(name, func(t *testing.T) {
			got := mapState(tt.active, tt.sub)
			if got != tt.want {
				t.Errorf("mapState(%q, %q) = %q, want %q", tt.active, tt.sub, got, tt.want)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager("test.service")
	if m.Unit() != "test.service" {
		t.Errorf("Unit() = %q, want test.service", m.Unit())
	}
}

func TestBuildJournalctlArgs(t *testing.T) {
	m := NewManager("minecraft.service")

	tests := []struct {
		name   string
		lines  int
		since  string
		follow bool
		want   []string
	}{
		{
			name:   "basic non-follow",
			lines:  0,
			since:  "",
			follow: false,
			want:   []string{"-u", "minecraft.service", "--no-pager", "--output", "short-iso"},
		},
		{
			name:   "with lines",
			lines:  50,
			since:  "",
			follow: false,
			want:   []string{"-u", "minecraft.service", "--no-pager", "--output", "short-iso", "-n", "50"},
		},
		{
			name:   "with since",
			lines:  0,
			since:  "1 hour ago",
			follow: false,
			want:   []string{"-u", "minecraft.service", "--no-pager", "--output", "short-iso", "--since", "1 hour ago"},
		},
		{
			name:   "follow mode",
			lines:  100,
			since:  "",
			follow: true,
			want:   []string{"-u", "minecraft.service", "--no-pager", "--follow", "--output", "cat", "-n", "100"},
		},
		{
			name:   "follow with since",
			lines:  10,
			since:  "today",
			follow: true,
			want:   []string{"-u", "minecraft.service", "--no-pager", "--follow", "--output", "cat", "-n", "10", "--since", "today"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.buildJournalctlArgs(tt.lines, tt.since, tt.follow)
			if len(got) != len(tt.want) {
				t.Fatalf("args length = %d, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d] = %q, want %q\ngot:  %v\nwant: %v", i, got[i], tt.want[i], got, tt.want)
				}
			}
		})
	}
}

func TestServiceStateConstants(t *testing.T) {
	// Verify the string values of state constants.
	if StateRunning != "running" {
		t.Errorf("StateRunning = %q", StateRunning)
	}
	if StateStopped != "stopped" {
		t.Errorf("StateStopped = %q", StateStopped)
	}
	if StateFailed != "failed" {
		t.Errorf("StateFailed = %q", StateFailed)
	}
	if StateStarting != "starting" {
		t.Errorf("StateStarting = %q", StateStarting)
	}
	if StateStopping != "stopping" {
		t.Errorf("StateStopping = %q", StateStopping)
	}
	if StateUnknown != "unknown" {
		t.Errorf("StateUnknown = %q", StateUnknown)
	}
}
