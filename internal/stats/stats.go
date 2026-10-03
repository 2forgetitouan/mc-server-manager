package stats

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// clockTicksPerSec is the standard value of sysconf(_SC_CLK_TCK) on Linux.
const clockTicksPerSec = 100

// ProcessStats holds resource usage information for a single process.
type ProcessStats struct {
	PID        int
	MemoryRSS  uint64 // bytes
	MemoryVMS  uint64 // bytes
	CPUPercent float64
	Uptime     time.Duration
	NumThreads int
}

// SystemStats holds system-wide resource information.
type SystemStats struct {
	TotalMemory     uint64 // bytes
	AvailableMemory uint64 // bytes
	TotalSwap       uint64 // bytes
	UsedSwap        uint64 // bytes
	DiskTotal       uint64 // bytes
	DiskFree        uint64 // bytes
	DiskPath        string
	LoadAvg1        float64
	LoadAvg5        float64
	LoadAvg15       float64
}

// GetProcessStats reads stats for a given PID from /proc.
func GetProcessStats(pid int) (*ProcessStats, error) {
	ps := &ProcessStats{PID: pid}

	// Read memory and thread info from /proc/<pid>/status.
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	statusData, err := os.ReadFile(statusPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", statusPath, err)
	}

	for _, line := range strings.Split(string(statusData), "\n") {
		fields := strings.SplitN(line, ":", 2)
		if len(fields) != 2 {
			continue
		}
		key := strings.TrimSpace(fields[0])
		val := strings.TrimSpace(fields[1])

		switch key {
		case "VmRSS":
			kb, err := parseKBValue(val)
			if err != nil {
				return nil, fmt.Errorf("parsing VmRSS from %s: %w", statusPath, err)
			}
			ps.MemoryRSS = kb * 1024
		case "VmSize":
			kb, err := parseKBValue(val)
			if err != nil {
				return nil, fmt.Errorf("parsing VmSize from %s: %w", statusPath, err)
			}
			ps.MemoryVMS = kb * 1024
		case "Threads":
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("parsing Threads from %s: %w", statusPath, err)
			}
			ps.NumThreads = n
		}
	}

	// Compute process uptime from /proc/<pid>/stat and /proc/uptime.
	uptime, err := processUptime(pid)
	if err != nil {
		return nil, fmt.Errorf("computing uptime for pid %d: %w", pid, err)
	}
	ps.Uptime = uptime

	return ps, nil
}

// MeasureCPU measures CPU usage over a short interval (~200ms).
// It reads /proc/[pid]/stat and /proc/stat twice and computes the delta.
func MeasureCPU(pid int) (float64, error) {
	procJiffies1, err := readProcessJiffies(pid)
	if err != nil {
		return 0, fmt.Errorf("reading initial process jiffies: %w", err)
	}
	totalJiffies1, err := readTotalCPUJiffies()
	if err != nil {
		return 0, fmt.Errorf("reading initial total jiffies: %w", err)
	}

	time.Sleep(200 * time.Millisecond)

	procJiffies2, err := readProcessJiffies(pid)
	if err != nil {
		return 0, fmt.Errorf("reading final process jiffies: %w", err)
	}
	totalJiffies2, err := readTotalCPUJiffies()
	if err != nil {
		return 0, fmt.Errorf("reading final total jiffies: %w", err)
	}

	deltaProc := procJiffies2 - procJiffies1
	deltaTotal := totalJiffies2 - totalJiffies1

	if deltaTotal == 0 {
		return 0, nil
	}

	numCPUs := runtime.NumCPU()
	cpuPercent := (float64(deltaProc) / float64(deltaTotal)) * 100.0 * float64(numCPUs)

	return cpuPercent, nil
}

// GetSystemStats reads system-wide stats from /proc and the filesystem.
func GetSystemStats(diskPath string) (*SystemStats, error) {
	ss := &SystemStats{DiskPath: diskPath}

	// Memory info from /proc/meminfo.
	memData, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, fmt.Errorf("reading /proc/meminfo: %w", err)
	}

	memFields := parseKeyValueFile(string(memData))

	if v, ok := memFields["MemTotal"]; ok {
		kb, err := parseKBValue(v)
		if err != nil {
			return nil, fmt.Errorf("parsing MemTotal: %w", err)
		}
		ss.TotalMemory = kb * 1024
	}
	if v, ok := memFields["MemAvailable"]; ok {
		kb, err := parseKBValue(v)
		if err != nil {
			return nil, fmt.Errorf("parsing MemAvailable: %w", err)
		}
		ss.AvailableMemory = kb * 1024
	}
	if v, ok := memFields["SwapTotal"]; ok {
		kb, err := parseKBValue(v)
		if err != nil {
			return nil, fmt.Errorf("parsing SwapTotal: %w", err)
		}
		ss.TotalSwap = kb * 1024
	}
	if v, ok := memFields["SwapFree"]; ok {
		kb, err := parseKBValue(v)
		if err != nil {
			return nil, fmt.Errorf("parsing SwapFree: %w", err)
		}
		ss.UsedSwap = ss.TotalSwap - (kb * 1024)
	}

	// Disk stats via statfs.
	if diskPath != "" {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(diskPath, &stat); err != nil {
			return nil, fmt.Errorf("statfs %s: %w", diskPath, err)
		}
		ss.DiskTotal = stat.Blocks * uint64(stat.Bsize)
		ss.DiskFree = stat.Bavail * uint64(stat.Bsize)
	}

	// Load averages from /proc/loadavg.
	loadData, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil, fmt.Errorf("reading /proc/loadavg: %w", err)
	}

	loadFields := strings.Fields(string(loadData))
	if len(loadFields) < 3 {
		return nil, fmt.Errorf("unexpected /proc/loadavg format: %q", string(loadData))
	}

	ss.LoadAvg1, err = strconv.ParseFloat(loadFields[0], 64)
	if err != nil {
		return nil, fmt.Errorf("parsing load average 1: %w", err)
	}
	ss.LoadAvg5, err = strconv.ParseFloat(loadFields[1], 64)
	if err != nil {
		return nil, fmt.Errorf("parsing load average 5: %w", err)
	}
	ss.LoadAvg15, err = strconv.ParseFloat(loadFields[2], 64)
	if err != nil {
		return nil, fmt.Errorf("parsing load average 15: %w", err)
	}

	return ss, nil
}

// FormatBytes formats a byte count into a human-readable string using binary
// units (KiB, MiB, GiB, TiB).
func FormatBytes(b uint64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
		tib = 1024 * gib
	)

	switch {
	case b >= tib:
		return fmt.Sprintf("%.1f TiB", float64(b)/float64(tib))
	case b >= gib:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(gib))
	case b >= mib:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(mib))
	case b >= kib:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(kib))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// FormatDuration formats a duration into a human-readable string such as
// "2d 5h 30m" or "45m 12s".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		return "0s"
	}

	totalSeconds := int(d.Seconds())
	days := totalSeconds / 86400
	hours := (totalSeconds % 86400) / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	var parts []string

	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 || days > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 || hours > 0 || days > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}

	// Only show seconds when the total duration is less than an hour.
	if days == 0 && hours == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}

	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}

// --- internal helpers ---

// parseKBValue parses a /proc value like "1234 kB" and returns the numeric
// part in kB.
func parseKBValue(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, " kB")
	s = strings.TrimSpace(s)
	return strconv.ParseUint(s, 10, 64)
}

// parseKeyValueFile parses a colon-separated key-value file (like /proc/meminfo).
func parseKeyValueFile(data string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return m
}

// processUptime calculates how long a process has been running.
func processUptime(pid int) (time.Duration, error) {
	// Read system uptime.
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/uptime: %w", err)
	}
	uptimeFields := strings.Fields(string(uptimeData))
	if len(uptimeFields) < 1 {
		return 0, fmt.Errorf("unexpected /proc/uptime format")
	}
	systemUptime, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parsing system uptime: %w", err)
	}

	// Read process start time from /proc/<pid>/stat (field 22, 1-indexed).
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", statPath, err)
	}

	starttime, err := extractStarttime(string(statData))
	if err != nil {
		return 0, fmt.Errorf("extracting starttime from %s: %w", statPath, err)
	}

	processStartSec := float64(starttime) / float64(clockTicksPerSec)
	processUptimeSec := systemUptime - processStartSec

	if processUptimeSec < 0 {
		processUptimeSec = 0
	}

	return time.Duration(processUptimeSec * float64(time.Second)), nil
}

// extractStarttime extracts field 22 (starttime) from /proc/<pid>/stat.
// The comm field (field 2) may contain spaces and parentheses, so we find the
// last ')' to skip past it safely.
func extractStarttime(statLine string) (uint64, error) {
	// Fields after comm start after the last ')'.
	idx := strings.LastIndex(statLine, ")")
	if idx < 0 || idx+2 >= len(statLine) {
		return 0, fmt.Errorf("malformed stat line: no closing parenthesis")
	}

	// Fields after ')' are space-separated. Field 3 is state, field 22 is
	// starttime. After the closing ')' the remaining fields start at index 3
	// (1-indexed), so starttime is at offset 22-3 = 19 within the remaining
	// fields.
	rest := strings.Fields(statLine[idx+2:])
	const starttimeOffset = 19 // field 22 minus field 3
	if len(rest) <= starttimeOffset {
		return 0, fmt.Errorf("stat line too short: need field 22, have %d fields after comm", len(rest))
	}

	return strconv.ParseUint(rest[starttimeOffset], 10, 64)
}

// readProcessJiffies reads utime + stime for a process from /proc/<pid>/stat.
func readProcessJiffies(pid int) (uint64, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", statPath, err)
	}

	idx := strings.LastIndex(string(data), ")")
	if idx < 0 || idx+2 >= len(data) {
		return 0, fmt.Errorf("malformed stat line in %s", statPath)
	}

	// After ')': field 3 = state, field 14 = utime (offset 11), field 15 = stime (offset 12).
	rest := strings.Fields(string(data)[idx+2:])
	const (
		utimeOffset = 11 // field 14 minus field 3
		stimeOffset = 12 // field 15 minus field 3
	)
	if len(rest) <= stimeOffset {
		return 0, fmt.Errorf("stat line too short in %s", statPath)
	}

	utime, err := strconv.ParseUint(rest[utimeOffset], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing utime from %s: %w", statPath, err)
	}
	stime, err := strconv.ParseUint(rest[stimeOffset], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing stime from %s: %w", statPath, err)
	}

	return utime + stime, nil
}

// readTotalCPUJiffies reads the aggregate CPU jiffies from the first line of
// /proc/stat.
func readTotalCPUJiffies() (uint64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/stat: %w", err)
	}

	// First line is "cpu  <user> <nice> <system> <idle> ...".
	firstLine, _, _ := strings.Cut(string(data), "\n")
	fields := strings.Fields(firstLine)
	if len(fields) < 2 || fields[0] != "cpu" {
		return 0, fmt.Errorf("unexpected /proc/stat format: %q", firstLine)
	}

	var total uint64
	for _, f := range fields[1:] {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing /proc/stat field %q: %w", f, err)
		}
		total += v
	}

	return total, nil
}
