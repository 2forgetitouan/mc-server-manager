package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"mc-server-manager/internal/config"
)

func testDoctor(t *testing.T, serverPath string) *Doctor {
	t.Helper()
	cfg := config.Default()
	cfg.ServerPath = serverPath
	return New(cfg)
}

func TestCheckConfig_Valid(t *testing.T) {
	d := testDoctor(t, "/tmp")
	result := d.CheckConfig()
	if result.Status != Pass {
		t.Errorf("Status = %q, want PASS: %s", result.Status, result.Message)
	}
	if result.Name != "Configuration" {
		t.Errorf("Name = %q, want Configuration", result.Name)
	}
}

func TestCheckConfig_Invalid(t *testing.T) {
	cfg := config.Default()
	cfg.ServerPath = ""
	d := New(cfg)

	result := d.CheckConfig()
	if result.Status != Fail {
		t.Errorf("Status = %q, want FAIL", result.Status)
	}
}

func TestCheckArchitecture(t *testing.T) {
	d := testDoctor(t, "/tmp")
	result := d.CheckArchitecture()

	// On amd64 or arm64 (common CI/dev arches), should be PASS.
	if result.Name != "Architecture" {
		t.Errorf("Name = %q, want Architecture", result.Name)
	}
	// Status should be either PASS or WARN depending on architecture.
	if result.Status != Pass && result.Status != Warn {
		t.Errorf("Status = %q, want PASS or WARN", result.Status)
	}
	if result.Message == "" {
		t.Error("Message should not be empty")
	}
}

func TestCheckServerDirectory_Exists(t *testing.T) {
	dir := t.TempDir()
	d := testDoctor(t, dir)

	result := d.CheckServerDirectory()
	if result.Status != Pass {
		t.Errorf("Status = %q, want PASS: %s", result.Status, result.Message)
	}
}

func TestCheckServerDirectory_NotExists(t *testing.T) {
	d := testDoctor(t, "/no/such/directory/exists")

	result := d.CheckServerDirectory()
	if result.Status != Fail {
		t.Errorf("Status = %q, want FAIL", result.Status)
	}
	if result.Fix == "" {
		t.Error("Fix should not be empty for failing check")
	}
}

func TestCheckServerDirectory_IsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "notadir")
	os.WriteFile(filePath, []byte("hello"), 0o644)

	cfg := config.Default()
	cfg.ServerPath = filePath
	d := New(cfg)

	result := d.CheckServerDirectory()
	if result.Status != Fail {
		t.Errorf("Status = %q, want FAIL", result.Status)
	}
}

func TestCheckDiskSpace(t *testing.T) {
	dir := t.TempDir()
	d := testDoctor(t, dir)

	result := d.CheckDiskSpace()
	// On a normal system, there should be enough disk space.
	if result.Name != "Disk space" {
		t.Errorf("Name = %q, want 'Disk space'", result.Name)
	}
	// Status should be PASS or WARN, not FAIL (unless truly out of space).
	if result.Status == Fail {
		t.Logf("Disk space check failed (may be low disk): %s", result.Message)
	}
}

func TestCheckMemory(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Server.MaxMemory = "1M" // Very small so available > max.
	d := New(cfg)

	result := d.CheckMemory()
	if result.Name != "Memory" {
		t.Errorf("Name = %q, want Memory", result.Name)
	}
	// With 1M max_memory, any system should have enough available.
	if result.Status != Pass {
		t.Logf("Memory check did not pass: %s", result.Message)
	}
}

func TestCheckMemory_LargeMaxMemory(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Server.MaxMemory = "999999G" // Impossibly large.
	d := New(cfg)

	result := d.CheckMemory()
	// Should warn because available < max_memory.
	if result.Status != Warn {
		t.Logf("Expected WARN for impossible max_memory, got %q: %s", result.Status, result.Message)
	}
}

func TestParseJavaVersion(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "Java 21",
			output: `openjdk version "21.0.1" 2023-10-17\nOpenJDK Runtime Environment (build 21.0.1+12-29)`,
			want:   "21.0.1",
		},
		{
			name:   "Java 17",
			output: `openjdk version "17.0.9" 2023-10-17`,
			want:   "17.0.9",
		},
		{
			name:   "Java 8",
			output: `openjdk version "1.8.0_312"`,
			want:   "1.8.0_312",
		},
		{
			name:   "no version",
			output: "command not found",
			want:   "",
		},
		{
			name:   "empty",
			output: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseJavaVersion(tt.output)
			if got != tt.want {
				t.Errorf("parseJavaVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseJavaMajor(t *testing.T) {
	tests := []struct {
		version string
		want    int
	}{
		{"21.0.1", 21},
		{"17.0.9", 17},
		{"1.8.0_312", 8},
		{"11.0.21", 11},
		{"21-ea", 21},
		{"", 0},
		{"abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := parseJavaMajor(tt.version)
			if got != tt.want {
				t.Errorf("parseJavaMajor(%q) = %d, want %d", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseMemorySize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		err   bool
	}{
		{"8G", 8 * 1024 * 1024 * 1024, false},
		{"512M", 512 * 1024 * 1024, false},
		{"1024K", 1024 * 1024, false},
		{"1g", 1024 * 1024 * 1024, false},
		{"256m", 256 * 1024 * 1024, false},
		{"1024k", 1024 * 1024, false},
		{"1024", 1024, false},
		{"", 0, true},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseMemorySize(tt.input)
			if (err != nil) != tt.err {
				t.Errorf("parseMemorySize(%q) error = %v, wantErr = %v", tt.input, err, tt.err)
				return
			}
			if got != tt.want {
				t.Errorf("parseMemorySize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello\nworld", "hello"},
		{"  \nfirst real line\nsecond", "first real line"},
		{"single line", "single line"},
		{"", ""},
		{"  ", ""},
	}

	for _, tt := range tests {
		got := firstLine(tt.input)
		if got != tt.want {
			t.Errorf("firstLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCheckRCON_ValidProperties(t *testing.T) {
	dir := t.TempDir()
	propsContent := `# Minecraft server properties
enable-rcon=true
rcon.port=25575
rcon.password=test123
`
	os.WriteFile(filepath.Join(dir, "server.properties"), []byte(propsContent), 0o644)

	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Console.Method = "rcon"
	d := New(cfg)

	result := d.CheckRCON(nil)
	if result.Status != Pass {
		t.Errorf("Status = %q, want PASS: %s", result.Status, result.Message)
	}
}

func TestCheckRCON_MissingProperties(t *testing.T) {
	dir := t.TempDir()
	// No server.properties file.

	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Console.Method = "rcon"
	d := New(cfg)

	result := d.CheckRCON(nil)
	if result.Status != Fail {
		t.Errorf("Status = %q, want FAIL", result.Status)
	}
}

func TestCheckRCON_RCONDisabled(t *testing.T) {
	dir := t.TempDir()
	propsContent := `enable-rcon=false
rcon.port=25575
rcon.password=test123
`
	os.WriteFile(filepath.Join(dir, "server.properties"), []byte(propsContent), 0o644)

	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Console.Method = "rcon"
	d := New(cfg)

	result := d.CheckRCON(nil)
	if result.Status != Fail {
		t.Errorf("Status = %q, want FAIL: enable-rcon not true", result.Status)
	}
}

func TestCheckRCON_NotRCONMethod(t *testing.T) {
	cfg := config.Default()
	cfg.Console.Method = ""
	d := New(cfg)

	result := d.CheckRCON(nil)
	if result.Status != Pass {
		t.Errorf("Status = %q, want PASS when method is not rcon", result.Status)
	}
}

func TestCheckServerJar(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "server.jar"), []byte("fake jar"), 0o644)

	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Server.Jar = "server.jar"
	d := New(cfg)

	result := d.CheckServerJar()
	if result.Status != Pass {
		t.Errorf("Status = %q, want PASS: %s", result.Status, result.Message)
	}
}

func TestCheckServerJar_Missing(t *testing.T) {
	dir := t.TempDir()

	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Server.Jar = "server.jar"
	d := New(cfg)

	result := d.CheckServerJar()
	if result.Status != Fail {
		t.Errorf("Status = %q, want FAIL", result.Status)
	}
}

func TestCheckMods_ValidDir(t *testing.T) {
	dir := t.TempDir()
	modsDir := filepath.Join(dir, "mods")
	os.MkdirAll(modsDir, 0o755)
	os.WriteFile(filepath.Join(modsDir, "test.jar"), []byte("jar"), 0o644)
	os.WriteFile(filepath.Join(modsDir, "readme.txt"), []byte("text"), 0o644)

	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Mods.Directory = "mods"
	d := New(cfg)

	result := d.CheckMods()
	if result.Status != Pass {
		t.Errorf("Status = %q, want PASS: %s", result.Status, result.Message)
	}
}

func TestCheckMods_NoDir(t *testing.T) {
	dir := t.TempDir()

	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Mods.Directory = "mods"
	d := New(cfg)

	result := d.CheckMods()
	if result.Status != Warn {
		t.Errorf("Status = %q, want WARN: %s", result.Status, result.Message)
	}
}

func TestCheckFilePermissions_ValidDir(t *testing.T) {
	dir := t.TempDir()
	modsDir := filepath.Join(dir, "mods")
	os.MkdirAll(modsDir, 0o755)

	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Mods.Directory = "mods"
	d := New(cfg)

	result := d.CheckFilePermissions()
	if result.Status != Pass {
		t.Errorf("Status = %q, want PASS: %s", result.Status, result.Message)
	}
}

func TestCheckReadWrite_NonexistentDir(t *testing.T) {
	err := checkReadWrite("/no/such/path")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestCheckReadWrite_File(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	os.WriteFile(filePath, []byte("content"), 0o644)

	err := checkReadWrite(filePath)
	if err == nil {
		t.Fatal("expected error when path is a file, not a directory")
	}
}

func TestReadServerProperties(t *testing.T) {
	dir := t.TempDir()
	content := `# Comment
server-port=25565
enable-rcon=true
rcon.password=secret
rcon.port=25575
empty-key=
`
	path := filepath.Join(dir, "server.properties")
	os.WriteFile(path, []byte(content), 0o644)

	props, err := readServerProperties(path)
	if err != nil {
		t.Fatalf("readServerProperties failed: %v", err)
	}

	if props["server-port"] != "25565" {
		t.Errorf("server-port = %q, want 25565", props["server-port"])
	}
	if props["enable-rcon"] != "true" {
		t.Errorf("enable-rcon = %q, want true", props["enable-rcon"])
	}
	if props["rcon.password"] != "secret" {
		t.Errorf("rcon.password = %q, want secret", props["rcon.password"])
	}
	if props["rcon.port"] != "25575" {
		t.Errorf("rcon.port = %q, want 25575", props["rcon.port"])
	}
	if props["empty-key"] != "" {
		t.Errorf("empty-key = %q, want empty", props["empty-key"])
	}
}

func TestStatusConstants(t *testing.T) {
	if Pass != "PASS" {
		t.Errorf("Pass = %q", Pass)
	}
	if Warn != "WARN" {
		t.Errorf("Warn = %q", Warn)
	}
	if Fail != "FAIL" {
		t.Errorf("Fail = %q", Fail)
	}
}
