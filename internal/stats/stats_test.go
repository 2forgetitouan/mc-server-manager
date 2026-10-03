package stats

import (
	"os"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name string
		b    uint64
		want string
	}{
		{"zero", 0, "0 B"},
		{"one byte", 1, "1 B"},
		{"1023 bytes", 1023, "1023 B"},
		{"1 KiB", 1024, "1.0 KiB"},
		{"1.5 KiB", 1536, "1.5 KiB"},
		{"1 MiB", 1024 * 1024, "1.0 MiB"},
		{"10 MiB", 10 * 1024 * 1024, "10.0 MiB"},
		{"1 GiB", 1024 * 1024 * 1024, "1.0 GiB"},
		{"2.5 GiB", uint64(2.5 * 1024 * 1024 * 1024), "2.5 GiB"},
		{"1 TiB", 1024 * 1024 * 1024 * 1024, "1.0 TiB"},
		{"large", 5 * 1024 * 1024 * 1024 * 1024, "5.0 TiB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytes(tt.b)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.b, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "0s"},
		{"negative", -5 * time.Second, "0s"},
		{"1 second", time.Second, "1s"},
		{"30 seconds", 30 * time.Second, "30s"},
		{"1 minute", time.Minute, "1m 0s"},
		{"5 minutes 30 seconds", 5*time.Minute + 30*time.Second, "5m 30s"},
		{"1 hour", time.Hour, "1h 0m"},
		{"1 hour 30 minutes", time.Hour + 30*time.Minute, "1h 30m"},
		{"1 day", 24 * time.Hour, "1d 0h 0m"},
		{"2 days 5 hours", 2*24*time.Hour + 5*time.Hour, "2d 5h 0m"},
		{"2 days 5 hours 30 minutes", 2*24*time.Hour + 5*time.Hour + 30*time.Minute, "2d 5h 30m"},
		{"59 minutes 59 seconds", 59*time.Minute + 59*time.Second, "59m 59s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestGetProcessStats(t *testing.T) {
	// Use the current process PID -- we know it exists.
	pid := os.Getpid()

	ps, err := GetProcessStats(pid)
	if err != nil {
		t.Fatalf("GetProcessStats(%d) failed: %v", pid, err)
	}

	if ps.PID != pid {
		t.Errorf("PID = %d, want %d", ps.PID, pid)
	}
	if ps.MemoryRSS == 0 {
		t.Error("MemoryRSS = 0, expected > 0")
	}
	if ps.MemoryVMS == 0 {
		t.Error("MemoryVMS = 0, expected > 0")
	}
	if ps.NumThreads < 1 {
		t.Errorf("NumThreads = %d, expected >= 1", ps.NumThreads)
	}
	if ps.Uptime < 0 {
		t.Errorf("Uptime = %v, expected >= 0", ps.Uptime)
	}
}

func TestGetProcessStats_InvalidPID(t *testing.T) {
	_, err := GetProcessStats(999999999)
	if err == nil {
		t.Fatal("expected error for invalid PID")
	}
}

func TestGetSystemStats(t *testing.T) {
	ss, err := GetSystemStats(".")
	if err != nil {
		t.Fatalf("GetSystemStats(\".\") failed: %v", err)
	}

	if ss.TotalMemory == 0 {
		t.Error("TotalMemory = 0, expected > 0")
	}
	if ss.AvailableMemory == 0 {
		t.Error("AvailableMemory = 0, expected > 0")
	}
	if ss.AvailableMemory > ss.TotalMemory {
		t.Errorf("AvailableMemory (%d) > TotalMemory (%d)", ss.AvailableMemory, ss.TotalMemory)
	}
	if ss.DiskTotal == 0 {
		t.Error("DiskTotal = 0, expected > 0")
	}
	if ss.DiskFree == 0 {
		t.Error("DiskFree = 0, expected > 0")
	}
	if ss.DiskFree > ss.DiskTotal {
		t.Errorf("DiskFree (%d) > DiskTotal (%d)", ss.DiskFree, ss.DiskTotal)
	}
	if ss.DiskPath != "." {
		t.Errorf("DiskPath = %q, want \".\"", ss.DiskPath)
	}
	// Load averages should be non-negative.
	if ss.LoadAvg1 < 0 {
		t.Errorf("LoadAvg1 = %f, expected >= 0", ss.LoadAvg1)
	}
	if ss.LoadAvg5 < 0 {
		t.Errorf("LoadAvg5 = %f, expected >= 0", ss.LoadAvg5)
	}
	if ss.LoadAvg15 < 0 {
		t.Errorf("LoadAvg15 = %f, expected >= 0", ss.LoadAvg15)
	}
}

func TestGetSystemStats_EmptyDiskPath(t *testing.T) {
	ss, err := GetSystemStats("")
	if err != nil {
		t.Fatalf("GetSystemStats(\"\") failed: %v", err)
	}

	// With empty disk path, disk stats should be zero.
	if ss.DiskTotal != 0 {
		t.Errorf("DiskTotal = %d, expected 0 for empty path", ss.DiskTotal)
	}
	if ss.DiskFree != 0 {
		t.Errorf("DiskFree = %d, expected 0 for empty path", ss.DiskFree)
	}
}

func TestMeasureCPU(t *testing.T) {
	pid := os.Getpid()

	cpuPercent, err := MeasureCPU(pid)
	if err != nil {
		t.Fatalf("MeasureCPU(%d) failed: %v", pid, err)
	}

	// CPU percent should be non-negative. It can be 0 if the process was
	// idle during the measurement window.
	if cpuPercent < 0 {
		t.Errorf("cpuPercent = %f, expected >= 0", cpuPercent)
	}
}

func TestMeasureCPU_InvalidPID(t *testing.T) {
	_, err := MeasureCPU(999999999)
	if err == nil {
		t.Fatal("expected error for invalid PID")
	}
}

func TestParseKBValue(t *testing.T) {
	tests := []struct {
		input string
		want  uint64
		err   bool
	}{
		{"1234 kB", 1234, false},
		{"0 kB", 0, false},
		{"  5678 kB  ", 5678, false},
		{"1024", 1024, false},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		got, err := parseKBValue(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("parseKBValue(%q) error = %v, wantErr = %v", tt.input, err, tt.err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseKBValue(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseKeyValueFile(t *testing.T) {
	data := `MemTotal:       16384000 kB
MemFree:         4096000 kB
MemAvailable:    8192000 kB
SwapTotal:       2048000 kB
SwapFree:        1024000 kB
`
	m := parseKeyValueFile(data)

	if got := m["MemTotal"]; got != "16384000 kB" {
		t.Errorf("MemTotal = %q, want %q", got, "16384000 kB")
	}
	if got := m["MemFree"]; got != "4096000 kB" {
		t.Errorf("MemFree = %q, want %q", got, "4096000 kB")
	}
	if got := m["MemAvailable"]; got != "8192000 kB" {
		t.Errorf("MemAvailable = %q, want %q", got, "8192000 kB")
	}
}

func TestExtractStarttime(t *testing.T) {
	// Simulated /proc/pid/stat line. The comm field is in parens.
	// Format: pid (comm) state ppid pgrp session tty_nr tpgid flags
	//   minflt cminflt majflt cmajflt utime stime cutime cstime priority nice
	//   num_threads itrealvalue starttime ...
	// Field 22 (1-indexed) is starttime.
	// After the closing ), the fields start at field 3.
	// starttime is at offset 19 (field 22 - field 3).
	statLine := "1234 (test process) S 1 1234 1234 0 -1 4194304 100 0 0 0 10 20 0 0 20 0 1 0 500 12345678 100 18446744073709551615"

	st, err := extractStarttime(statLine)
	if err != nil {
		t.Fatalf("extractStarttime failed: %v", err)
	}
	if st != 500 {
		t.Errorf("starttime = %d, want 500", st)
	}
}

func TestExtractStarttime_WithSpacesInComm(t *testing.T) {
	// comm field can have spaces and parens.
	statLine := "1234 (test (special) process) S 1 1234 1234 0 -1 4194304 100 0 0 0 10 20 0 0 20 0 1 0 999 12345678 100 18446744073709551615"

	st, err := extractStarttime(statLine)
	if err != nil {
		t.Fatalf("extractStarttime failed: %v", err)
	}
	if st != 999 {
		t.Errorf("starttime = %d, want 999", st)
	}
}

func TestExtractStarttime_Malformed(t *testing.T) {
	_, err := extractStarttime("no closing paren here")
	if err == nil {
		t.Fatal("expected error for malformed stat line")
	}
}
