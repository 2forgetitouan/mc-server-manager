package doctor

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mc-server-manager/internal/config"
	"mc-server-manager/internal/systemd"
)

// Status represents the outcome of a single diagnostic check.
type Status string

const (
	Pass Status = "PASS"
	Warn Status = "WARN"
	Fail Status = "FAIL"
)

// CheckResult holds the outcome and details for one diagnostic check.
type CheckResult struct {
	Name    string
	Status  Status
	Message string
	Fix     string // suggested remediation; empty when not applicable
}

// Doctor runs diagnostic checks against the server environment.
type Doctor struct {
	cfg  *config.Config
	sysd *systemd.Manager
}

// New creates a Doctor for the given configuration.
func New(cfg *config.Config) *Doctor {
	return &Doctor{
		cfg:  cfg,
		sysd: systemd.NewManager(cfg.Systemd.Unit),
	}
}

// RunAll executes every diagnostic check and returns the collected results.
func (d *Doctor) RunAll(ctx context.Context) []CheckResult {
	return []CheckResult{
		d.CheckConfig(),
		d.CheckJava(ctx),
		d.CheckArchitecture(),
		d.CheckServerDirectory(),
		d.CheckServerJar(),
		d.CheckMods(),
		d.CheckFilePermissions(),
		d.CheckSystemdUnit(ctx),
		d.CheckSystemdService(ctx),
		d.CheckJournald(ctx),
		d.CheckRCON(ctx),
		d.CheckPort(),
		d.CheckDiskSpace(),
		d.CheckMemory(),
		d.CheckModrinthAccess(ctx),
	}
}

// CheckJava verifies that the configured Java binary exists and is version 17+.
func (d *Doctor) CheckJava(ctx context.Context) CheckResult {
	javaPath := d.cfg.Server.Java
	if javaPath == "" {
		javaPath = "java"
	}

	cmd := exec.CommandContext(ctx, javaPath, "-version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return CheckResult{
			Name:    "Java",
			Status:  Fail,
			Message: fmt.Sprintf("Java not found at %s: %v", javaPath, err),
			Fix:     "Install Java 21+ or set server.java in config to the correct path",
		}
	}

	version := parseJavaVersion(string(output))
	if version == "" {
		return CheckResult{
			Name:    "Java",
			Status:  Warn,
			Message: fmt.Sprintf("Java found but could not parse version from: %s", firstLine(string(output))),
		}
	}

	major := parseJavaMajor(version)
	if major < 17 {
		return CheckResult{
			Name:    "Java",
			Status:  Fail,
			Message: fmt.Sprintf("Java %s detected, but 17+ is required", version),
			Fix:     "Install Java 17 or newer",
		}
	}

	return CheckResult{
		Name:    "Java",
		Status:  Pass,
		Message: fmt.Sprintf("Java %s (%s)", version, javaPath),
	}
}

// CheckArchitecture reports the CPU architecture and warns on unusual targets.
func (d *Doctor) CheckArchitecture() CheckResult {
	arch := runtime.GOARCH
	switch arch {
	case "amd64", "arm64":
		return CheckResult{
			Name:    "Architecture",
			Status:  Pass,
			Message: arch,
		}
	default:
		return CheckResult{
			Name:    "Architecture",
			Status:  Warn,
			Message: fmt.Sprintf("%s (untested architecture)", arch),
		}
	}
}

// CheckServerDirectory verifies the server path exists and is a directory.
func (d *Doctor) CheckServerDirectory() CheckResult {
	info, err := os.Stat(d.cfg.ServerPath)
	if err != nil {
		return CheckResult{
			Name:    "Server directory",
			Status:  Fail,
			Message: fmt.Sprintf("%s: %v", d.cfg.ServerPath, err),
			Fix:     "Create the directory or update server_path in config",
		}
	}
	if !info.IsDir() {
		return CheckResult{
			Name:    "Server directory",
			Status:  Fail,
			Message: fmt.Sprintf("%s exists but is not a directory", d.cfg.ServerPath),
			Fix:     "server_path must point to a directory",
		}
	}
	return CheckResult{
		Name:    "Server directory",
		Status:  Pass,
		Message: d.cfg.ServerPath,
	}
}

// CheckServerJar checks that the server jar file exists at the expected path.
func (d *Doctor) CheckServerJar() CheckResult {
	jarPath := filepath.Join(d.cfg.ServerPath, d.cfg.Server.Jar)
	info, err := os.Stat(jarPath)
	if err != nil {
		return CheckResult{
			Name:    "Server JAR",
			Status:  Fail,
			Message: fmt.Sprintf("%s not found: %v", jarPath, err),
			Fix:     fmt.Sprintf("Place the server jar at %s or update server.jar in config", jarPath),
		}
	}
	return CheckResult{
		Name:    "Server JAR",
		Status:  Pass,
		Message: fmt.Sprintf("%s (%.1f MB)", d.cfg.Server.Jar, float64(info.Size())/(1024*1024)),
	}
}

// CheckMods verifies the mods directory exists and counts .jar files.
func (d *Doctor) CheckMods() CheckResult {
	modsDir := d.cfg.ModsPath()
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:    "Mods directory",
				Status:  Warn,
				Message: fmt.Sprintf("%s does not exist", modsDir),
				Fix:     "Create the mods directory or update mods.directory in config",
			}
		}
		return CheckResult{
			Name:    "Mods directory",
			Status:  Fail,
			Message: fmt.Sprintf("Cannot read %s: %v", modsDir, err),
		}
	}

	var jarCount int
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			jarCount++
		}
	}

	return CheckResult{
		Name:    "Mods directory",
		Status:  Pass,
		Message: fmt.Sprintf("%s (%d jar files)", modsDir, jarCount),
	}
}

// CheckSystemdUnit verifies the systemd unit file is known to systemd.
func (d *Doctor) CheckSystemdUnit(ctx context.Context) CheckResult {
	if !d.sysd.UnitExists(ctx) {
		return CheckResult{
			Name:    "Systemd unit",
			Status:  Fail,
			Message: fmt.Sprintf("Unit %s not found", d.cfg.Systemd.Unit),
			Fix:     "Install the systemd unit file and run 'systemctl daemon-reload'",
		}
	}
	return CheckResult{
		Name:    "Systemd unit",
		Status:  Pass,
		Message: fmt.Sprintf("%s is registered", d.cfg.Systemd.Unit),
	}
}

// CheckSystemdService checks the current state of the systemd service.
func (d *Doctor) CheckSystemdService(ctx context.Context) CheckResult {
	status, err := d.sysd.Status(ctx)
	if err != nil {
		return CheckResult{
			Name:    "Systemd service",
			Status:  Warn,
			Message: fmt.Sprintf("Could not query service status: %v", err),
		}
	}

	switch status.State {
	case systemd.StateRunning:
		msg := fmt.Sprintf("Running (PID %d)", status.PID)
		if !status.ActiveSince.IsZero() {
			msg += fmt.Sprintf(", up since %s", status.ActiveSince.Format(time.RFC3339))
		}
		return CheckResult{
			Name:    "Systemd service",
			Status:  Pass,
			Message: msg,
		}
	case systemd.StateFailed:
		return CheckResult{
			Name:    "Systemd service",
			Status:  Fail,
			Message: fmt.Sprintf("Service is in failed state (exit code: %s)", status.ExitCode),
			Fix:     "Check logs with 'mc logs' and restart with 'mc start'",
		}
	case systemd.StateStopped:
		return CheckResult{
			Name:    "Systemd service",
			Status:  Warn,
			Message: "Service is stopped",
		}
	default:
		return CheckResult{
			Name:    "Systemd service",
			Status:  Warn,
			Message: fmt.Sprintf("Service state: %s (%s)", status.State, status.SubState),
		}
	}
}

// CheckJournald verifies that journald log access works for this unit.
func (d *Doctor) CheckJournald(ctx context.Context) CheckResult {
	lines, err := d.sysd.LogLines(ctx, 1, "")
	if err != nil {
		return CheckResult{
			Name:    "Journald access",
			Status:  Fail,
			Message: fmt.Sprintf("Cannot read journal: %v", err),
			Fix:     "Ensure the current user has permission to read journald logs",
		}
	}
	if len(lines) == 0 {
		return CheckResult{
			Name:    "Journald access",
			Status:  Warn,
			Message: "Journal is accessible but contains no entries for this unit",
		}
	}
	return CheckResult{
		Name:    "Journald access",
		Status:  Pass,
		Message: "Journal is readable",
	}
}

// CheckFilePermissions verifies the current user can read and write key
// directories (server path and mods directory).
func (d *Doctor) CheckFilePermissions() CheckResult {
	checks := []struct {
		label string
		path  string
	}{
		{"server directory", d.cfg.ServerPath},
		{"mods directory", d.cfg.ModsPath()},
	}

	var issues []string
	for _, c := range checks {
		if err := checkReadWrite(c.path); err != nil {
			issues = append(issues, fmt.Sprintf("%s (%s): %v", c.label, c.path, err))
		}
	}

	if len(issues) > 0 {
		return CheckResult{
			Name:    "File permissions",
			Status:  Fail,
			Message: strings.Join(issues, "; "),
			Fix:     "Fix ownership/permissions so the current user can read and write these paths",
		}
	}

	return CheckResult{
		Name:    "File permissions",
		Status:  Pass,
		Message: "Server and mods directories are readable and writable",
	}
}

// CheckRCON verifies that RCON is properly configured in server.properties.
// It does NOT attempt to authenticate (the server might be stopped).
func (d *Doctor) CheckRCON(ctx context.Context) CheckResult {
	if d.cfg.Console.Method != "rcon" {
		return CheckResult{
			Name:    "RCON",
			Status:  Pass,
			Message: "RCON not configured (console method is not rcon)",
		}
	}

	propsPath := d.cfg.ServerPropertiesPath()
	props, err := readServerProperties(propsPath)
	if err != nil {
		return CheckResult{
			Name:    "RCON",
			Status:  Fail,
			Message: fmt.Sprintf("Cannot read server.properties: %v", err),
			Fix:     "Ensure server.properties exists and is readable",
		}
	}

	var issues []string

	if props["enable-rcon"] != "true" {
		issues = append(issues, "enable-rcon is not set to true")
	}
	if props["rcon.port"] == "" {
		issues = append(issues, "rcon.port is not set")
	}
	if props["rcon.password"] == "" {
		issues = append(issues, "rcon.password is empty or not set")
	}

	if len(issues) > 0 {
		return CheckResult{
			Name:    "RCON",
			Status:  Fail,
			Message: strings.Join(issues, "; "),
			Fix:     "Set enable-rcon=true, rcon.port, and rcon.password in server.properties",
		}
	}

	return CheckResult{
		Name:    "RCON",
		Status:  Pass,
		Message: fmt.Sprintf("RCON enabled on port %s", props["rcon.port"]),
	}
}

// CheckPort reports whether the Minecraft server port is currently in use.
func (d *Doctor) CheckPort() CheckResult {
	port := d.cfg.Server.Port
	addr := fmt.Sprintf(":%d", port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Port is already in use, likely by our server.
		return CheckResult{
			Name:    "Server port",
			Status:  Pass,
			Message: fmt.Sprintf("Port %d is in use (server may be running)", port),
		}
	}
	ln.Close()

	return CheckResult{
		Name:    "Server port",
		Status:  Warn,
		Message: fmt.Sprintf("Port %d is not in use (server may be stopped)", port),
	}
}

// CheckDiskSpace checks available disk space on the server path volume.
// Warns if < 5 GB free; fails if < 1 GB free.
func (d *Doctor) CheckDiskSpace() CheckResult {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(d.cfg.ServerPath, &stat); err != nil {
		return CheckResult{
			Name:    "Disk space",
			Status:  Warn,
			Message: fmt.Sprintf("Cannot check disk space: %v", err),
		}
	}

	availableBytes := stat.Bavail * uint64(stat.Bsize)
	availableGB := float64(availableBytes) / (1024 * 1024 * 1024)

	if availableGB < 1 {
		return CheckResult{
			Name:    "Disk space",
			Status:  Fail,
			Message: fmt.Sprintf("%.1f GB available (critically low)", availableGB),
			Fix:     "Free up disk space on the server volume",
		}
	}
	if availableGB < 5 {
		return CheckResult{
			Name:    "Disk space",
			Status:  Warn,
			Message: fmt.Sprintf("%.1f GB available (low)", availableGB),
			Fix:     "Consider freeing disk space",
		}
	}

	return CheckResult{
		Name:    "Disk space",
		Status:  Pass,
		Message: fmt.Sprintf("%.1f GB available", availableGB),
	}
}

// CheckMemory checks available system memory against the configured maximum
// JVM heap size.
func (d *Doctor) CheckMemory() CheckResult {
	availableKB, err := readMemAvailable()
	if err != nil {
		return CheckResult{
			Name:    "Memory",
			Status:  Warn,
			Message: fmt.Sprintf("Cannot check memory: %v", err),
		}
	}

	availableBytes := availableKB * 1024
	availableGB := float64(availableBytes) / (1024 * 1024 * 1024)

	maxBytes, err := parseMemorySize(d.cfg.Server.MaxMemory)
	if err != nil {
		return CheckResult{
			Name:    "Memory",
			Status:  Pass,
			Message: fmt.Sprintf("%.1f GB available (could not parse max_memory config: %v)", availableGB, err),
		}
	}

	if availableBytes < maxBytes {
		return CheckResult{
			Name:   "Memory",
			Status: Warn,
			Message: fmt.Sprintf("%.1f GB available, but server max_memory is %s",
				availableGB, d.cfg.Server.MaxMemory),
			Fix: "Increase system memory or reduce server.max_memory",
		}
	}

	return CheckResult{
		Name:    "Memory",
		Status:  Pass,
		Message: fmt.Sprintf("%.1f GB available (max_memory: %s)", availableGB, d.cfg.Server.MaxMemory),
	}
}

// CheckConfig validates the configuration file for internal consistency.
func (d *Doctor) CheckConfig() CheckResult {
	if err := d.cfg.Validate(); err != nil {
		return CheckResult{
			Name:    "Configuration",
			Status:  Fail,
			Message: fmt.Sprintf("Invalid config: %v", err),
			Fix:     "Fix the listed issues in your config file",
		}
	}
	return CheckResult{
		Name:    "Configuration",
		Status:  Pass,
		Message: "Configuration is valid",
	}
}

// CheckModrinthAccess checks connectivity to the Modrinth API.
func (d *Doctor) CheckModrinthAccess(ctx context.Context) CheckResult {
	if !d.cfg.Modrinth.Enabled {
		return CheckResult{
			Name:    "Modrinth API",
			Status:  Pass,
			Message: "Modrinth integration is disabled",
		}
	}

	timeout := time.Duration(d.cfg.Modrinth.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.modrinth.com/", nil)
	if err != nil {
		return CheckResult{
			Name:    "Modrinth API",
			Status:  Fail,
			Message: fmt.Sprintf("Failed to create request: %v", err),
		}
	}
	req.Header.Set("User-Agent", d.cfg.Modrinth.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{
			Name:    "Modrinth API",
			Status:  Fail,
			Message: fmt.Sprintf("Cannot reach api.modrinth.com: %v", err),
			Fix:     "Check network connectivity and DNS resolution",
		}
	}
	resp.Body.Close()

	if resp.StatusCode >= 500 {
		return CheckResult{
			Name:    "Modrinth API",
			Status:  Warn,
			Message: fmt.Sprintf("Modrinth returned HTTP %d (server-side issue)", resp.StatusCode),
		}
	}

	return CheckResult{
		Name:    "Modrinth API",
		Status:  Pass,
		Message: fmt.Sprintf("Reachable (HTTP %d)", resp.StatusCode),
	}
}

// ---------- helpers ----------

// parseJavaVersion extracts the quoted version string from `java -version`
// output (which is written to stderr).
func parseJavaVersion(output string) string {
	for _, line := range strings.Split(output, "\n") {
		start := strings.IndexByte(line, '"')
		if start < 0 {
			continue
		}
		end := strings.IndexByte(line[start+1:], '"')
		if end < 0 {
			continue
		}
		return line[start+1 : start+1+end]
	}
	return ""
}

// parseJavaMajor returns the major version number from a Java version string.
// It handles both old-style "1.8.0_312" (returns 8) and new-style "17.0.1"
// (returns 17).
func parseJavaMajor(version string) int {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) == 0 {
		return 0
	}

	// Extract leading digits from the first component (handles "21-ea" etc.).
	numStr := parts[0]
	for i, c := range numStr {
		if c < '0' || c > '9' {
			numStr = numStr[:i]
			break
		}
	}

	major, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}

	// Old versioning scheme: "1.8.x" means Java 8.
	if major == 1 && len(parts) > 1 {
		minor, err := strconv.Atoi(parts[1])
		if err != nil {
			return major
		}
		return minor
	}

	return major
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return strings.TrimSpace(s)
}

// parseMemorySize converts a JVM-style size string (e.g., "8G", "512M",
// "1024K") to bytes.
func parseMemorySize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty memory size")
	}

	multiplier := int64(1)
	last := s[len(s)-1]
	switch last {
	case 'G', 'g':
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case 'M', 'm':
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	case 'K', 'k':
		multiplier = 1024
		s = s[:len(s)-1]
	}

	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory size %q: %w", s, err)
	}

	return val * multiplier, nil
}

// readMemAvailable reads the MemAvailable value from /proc/meminfo in
// kilobytes.
func readMemAvailable() (int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("unexpected format: %s", line)
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing MemAvailable: %w", err)
		}
		return kb, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return 0, fmt.Errorf("MemAvailable not found in /proc/meminfo")
}

// readServerProperties parses a Minecraft server.properties file into a
// key-value map.
func readServerProperties(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	props := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		props[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return props, scanner.Err()
}

// checkReadWrite tests that the current user can read from and write to a
// directory by attempting to create and immediately remove a temporary file.
func checkReadWrite(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("does not exist")
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}

	// Attempt to create a temp file to verify write access.
	tmp := filepath.Join(path, ".mc-doctor-check")
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("not writable: %w", err)
	}
	f.Close()
	os.Remove(tmp)

	return nil
}
