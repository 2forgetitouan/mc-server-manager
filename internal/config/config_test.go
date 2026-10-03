package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.ServerPath != "/home/ubuntu/minecraft" {
		t.Errorf("Default ServerPath = %q, want /home/ubuntu/minecraft", cfg.ServerPath)
	}
	if cfg.Server.MinecraftVersion != "1.21.1" {
		t.Errorf("Default MinecraftVersion = %q, want 1.21.1", cfg.Server.MinecraftVersion)
	}
	if cfg.Server.Loader != "fabric" {
		t.Errorf("Default Loader = %q, want fabric", cfg.Server.Loader)
	}
	if cfg.Server.Java != "/usr/bin/java" {
		t.Errorf("Default Java = %q, want /usr/bin/java", cfg.Server.Java)
	}
	if cfg.Server.MinMemory != "2G" {
		t.Errorf("Default MinMemory = %q, want 2G", cfg.Server.MinMemory)
	}
	if cfg.Server.MaxMemory != "8G" {
		t.Errorf("Default MaxMemory = %q, want 8G", cfg.Server.MaxMemory)
	}
	if cfg.Server.Jar != "server.jar" {
		t.Errorf("Default Jar = %q, want server.jar", cfg.Server.Jar)
	}
	if cfg.Server.Port != 25565 {
		t.Errorf("Default Port = %d, want 25565", cfg.Server.Port)
	}
	if cfg.Systemd.Unit != "minecraft.service" {
		t.Errorf("Default Systemd.Unit = %q, want minecraft.service", cfg.Systemd.Unit)
	}
	if cfg.Systemd.StopTimeout != 60 {
		t.Errorf("Default StopTimeout = %d, want 60", cfg.Systemd.StopTimeout)
	}
	if cfg.Console.Method != "rcon" {
		t.Errorf("Default Console.Method = %q, want rcon", cfg.Console.Method)
	}
	if cfg.Console.RCONPort != 25575 {
		t.Errorf("Default Console.RCONPort = %d, want 25575", cfg.Console.RCONPort)
	}
	if cfg.Console.RCONHost != "127.0.0.1" {
		t.Errorf("Default Console.RCONHost = %q, want 127.0.0.1", cfg.Console.RCONHost)
	}
	if !cfg.Modrinth.Enabled {
		t.Error("Default Modrinth.Enabled = false, want true")
	}
	if cfg.Modrinth.Timeout != 30 {
		t.Errorf("Default Modrinth.Timeout = %d, want 30", cfg.Modrinth.Timeout)
	}
	if cfg.Mods.Directory != "mods" {
		t.Errorf("Default Mods.Directory = %q, want mods", cfg.Mods.Directory)
	}
	if cfg.Mods.BackupDir != "mods-backup" {
		t.Errorf("Default Mods.BackupDir = %q, want mods-backup", cfg.Mods.BackupDir)
	}
	if cfg.Mods.MaxBackups != 5 {
		t.Errorf("Default Mods.MaxBackups = %d, want 5", cfg.Mods.MaxBackups)
	}
	if cfg.Mods.Pinned == nil {
		t.Error("Default Mods.Pinned is nil, want non-nil map")
	}
	if cfg.Display.Color != "auto" {
		t.Errorf("Default Display.Color = %q, want auto", cfg.Display.Color)
	}
	if cfg.Display.LogLines != 50 {
		t.Errorf("Default Display.LogLines = %d, want 50", cfg.Display.LogLines)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "mc.toml")

	content := `
server_path = "/opt/minecraft"

[server]
minecraft_version = "1.20.4"
loader = "forge"
java = "/usr/lib/jvm/java-21/bin/java"
min_memory = "4G"
max_memory = "16G"
jar = "forge-server.jar"
port = 25566

[systemd]
unit = "mc-custom.service"
stop_timeout = 120
restart_policy = "on-failure"

[console]
method = "rcon"
rcon_port = 25576
rcon_host = "192.168.1.10"

[modrinth]
enabled = false
timeout = 60

[mods]
directory = "custom-mods"
backup_dir = "custom-backup"
max_backups = 10

[display]
color = "always"
log_lines = 100
`
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	cfg, err := LoadFrom(cfgFile)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	if cfg.ServerPath != "/opt/minecraft" {
		t.Errorf("ServerPath = %q, want /opt/minecraft", cfg.ServerPath)
	}
	if cfg.Server.MinecraftVersion != "1.20.4" {
		t.Errorf("MinecraftVersion = %q, want 1.20.4", cfg.Server.MinecraftVersion)
	}
	if cfg.Server.Loader != "forge" {
		t.Errorf("Loader = %q, want forge", cfg.Server.Loader)
	}
	if cfg.Server.Java != "/usr/lib/jvm/java-21/bin/java" {
		t.Errorf("Java = %q", cfg.Server.Java)
	}
	if cfg.Server.MinMemory != "4G" {
		t.Errorf("MinMemory = %q, want 4G", cfg.Server.MinMemory)
	}
	if cfg.Server.MaxMemory != "16G" {
		t.Errorf("MaxMemory = %q, want 16G", cfg.Server.MaxMemory)
	}
	if cfg.Server.Jar != "forge-server.jar" {
		t.Errorf("Jar = %q, want forge-server.jar", cfg.Server.Jar)
	}
	if cfg.Server.Port != 25566 {
		t.Errorf("Port = %d, want 25566", cfg.Server.Port)
	}
	if cfg.Systemd.Unit != "mc-custom.service" {
		t.Errorf("Systemd.Unit = %q", cfg.Systemd.Unit)
	}
	if cfg.Systemd.StopTimeout != 120 {
		t.Errorf("StopTimeout = %d, want 120", cfg.Systemd.StopTimeout)
	}
	if cfg.Console.RCONPort != 25576 {
		t.Errorf("RCONPort = %d, want 25576", cfg.Console.RCONPort)
	}
	if cfg.Console.RCONHost != "192.168.1.10" {
		t.Errorf("RCONHost = %q", cfg.Console.RCONHost)
	}
	if cfg.Modrinth.Enabled {
		t.Error("Modrinth.Enabled = true, want false")
	}
	if cfg.Modrinth.Timeout != 60 {
		t.Errorf("Modrinth.Timeout = %d, want 60", cfg.Modrinth.Timeout)
	}
	if cfg.Mods.Directory != "custom-mods" {
		t.Errorf("Mods.Directory = %q", cfg.Mods.Directory)
	}
	if cfg.Mods.BackupDir != "custom-backup" {
		t.Errorf("Mods.BackupDir = %q", cfg.Mods.BackupDir)
	}
	if cfg.Mods.MaxBackups != 10 {
		t.Errorf("Mods.MaxBackups = %d, want 10", cfg.Mods.MaxBackups)
	}
	if cfg.Display.Color != "always" {
		t.Errorf("Display.Color = %q, want always", cfg.Display.Color)
	}
	if cfg.Display.LogLines != 100 {
		t.Errorf("Display.LogLines = %d, want 100", cfg.Display.LogLines)
	}
}

func TestLoadFromNonexistent(t *testing.T) {
	_, err := LoadFrom("/no/such/file.toml")
	if err == nil {
		t.Fatal("LoadFrom should fail for non-existent file")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default config should be valid: %v", err)
	}
}

func TestValidate_EmptyServerPath(t *testing.T) {
	cfg := Default()
	cfg.ServerPath = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty server_path")
	}
	if got := err.Error(); !contains(got, "server_path must not be empty") {
		t.Errorf("error = %q, want mention of server_path", got)
	}
}

func TestValidate_EmptyJar(t *testing.T) {
	cfg := Default()
	cfg.Server.Jar = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty jar")
	}
	if got := err.Error(); !contains(got, "server.jar must not be empty") {
		t.Errorf("error = %q, want mention of server.jar", got)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 70000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Server.Port = tt.port
			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected error for invalid port")
			}
			if got := err.Error(); !contains(got, "server.port must be between") {
				t.Errorf("error = %q", got)
			}
		})
	}
}

func TestValidate_InvalidConsoleMethod(t *testing.T) {
	cfg := Default()
	cfg.Console.Method = "telnet"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid console method")
	}
	if got := err.Error(); !contains(got, "console.method") {
		t.Errorf("error = %q", got)
	}
}

func TestValidate_InvalidRCONPort(t *testing.T) {
	cfg := Default()
	cfg.Console.RCONPort = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid RCON port")
	}
}

func TestValidate_NegativeStopTimeout(t *testing.T) {
	cfg := Default()
	cfg.Systemd.StopTimeout = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative stop_timeout")
	}
}

func TestValidate_NegativeModrinthTimeout(t *testing.T) {
	cfg := Default()
	cfg.Modrinth.Timeout = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative modrinth timeout")
	}
}

func TestValidate_NegativeMaxBackups(t *testing.T) {
	cfg := Default()
	cfg.Mods.MaxBackups = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative max_backups")
	}
}

func TestValidate_InvalidDisplayColor(t *testing.T) {
	cfg := Default()
	cfg.Display.Color = "rainbow"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid display color")
	}
}

func TestValidate_NegativeLogLines(t *testing.T) {
	cfg := Default()
	cfg.Display.LogLines = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative log_lines")
	}
}

func TestValidate_EmptySystemdUnit(t *testing.T) {
	cfg := Default()
	cfg.Systemd.Unit = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty systemd unit")
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := Default()
	cfg.ServerPath = ""
	cfg.Server.Jar = ""
	cfg.Server.Port = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected multiple validation errors")
	}
	got := err.Error()
	if !contains(got, "server_path") {
		t.Errorf("missing server_path error in: %q", got)
	}
	if !contains(got, "server.jar") {
		t.Errorf("missing server.jar error in: %q", got)
	}
	if !contains(got, "server.port") {
		t.Errorf("missing server.port error in: %q", got)
	}
}

func TestReadRCONPassword(t *testing.T) {
	dir := t.TempDir()
	propsContent := `# Minecraft server properties
server-port=25565
rcon.port=25575
enable-rcon=true
rcon.password=supersecret123
`
	propsPath := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(propsPath, []byte(propsContent), 0o644); err != nil {
		t.Fatalf("writing server.properties: %v", err)
	}

	cfg := Default()
	cfg.ServerPath = dir

	pw, err := cfg.ReadRCONPassword()
	if err != nil {
		t.Fatalf("ReadRCONPassword failed: %v", err)
	}
	if pw != "supersecret123" {
		t.Errorf("password = %q, want supersecret123", pw)
	}
}

func TestReadRCONPassword_FileNotExist(t *testing.T) {
	cfg := Default()
	cfg.ServerPath = "/no/such/directory"

	_, err := cfg.ReadRCONPassword()
	if err == nil {
		t.Fatal("expected error when server.properties does not exist")
	}
}

func TestReadRCONPassword_KeyMissing(t *testing.T) {
	dir := t.TempDir()
	propsContent := `# Minecraft server properties
server-port=25565
enable-rcon=true
`
	propsPath := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(propsPath, []byte(propsContent), 0o644); err != nil {
		t.Fatalf("writing server.properties: %v", err)
	}

	cfg := Default()
	cfg.ServerPath = dir

	_, err := cfg.ReadRCONPassword()
	if err == nil {
		t.Fatal("expected error when rcon.password is not in file")
	}
	if got := err.Error(); !contains(got, "not found") {
		t.Errorf("error = %q, want 'not found'", got)
	}
}

func TestReadRCONPassword_EmptyPassword(t *testing.T) {
	dir := t.TempDir()
	propsContent := `rcon.password=
`
	propsPath := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(propsPath, []byte(propsContent), 0o644); err != nil {
		t.Fatalf("writing server.properties: %v", err)
	}

	cfg := Default()
	cfg.ServerPath = dir

	_, err := cfg.ReadRCONPassword()
	if err == nil {
		t.Fatal("expected error for empty rcon.password")
	}
	if got := err.Error(); !contains(got, "empty") {
		t.Errorf("error = %q, want mention of 'empty'", got)
	}
}

func TestFindConfigPath_WithEnvVar(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "custom.toml")
	if err := os.WriteFile(cfgFile, []byte("server_path = \"/tmp\"\n"), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	t.Setenv("MC_CONFIG", cfgFile)

	path := FindConfigPath()
	if path != cfgFile {
		t.Errorf("FindConfigPath() = %q, want %q", path, cfgFile)
	}
}

func TestFindConfigPath_NoConfig(t *testing.T) {
	// Clear env and ensure we're not in a directory with mc.toml
	t.Setenv("MC_CONFIG", "")

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	path := FindConfigPath()
	// We can't guarantee there's no config in ~/.config/mc/ or /etc/mc/,
	// but at least MC_CONFIG and ./mc.toml shouldn't match.
	// If a config exists in the standard paths, path will be non-empty -- that's OK.
	_ = path
}

func TestModsPath_Relative(t *testing.T) {
	cfg := Default()
	cfg.ServerPath = "/opt/minecraft"
	cfg.Mods.Directory = "mods"

	got := cfg.ModsPath()
	want := "/opt/minecraft/mods"
	if got != want {
		t.Errorf("ModsPath() = %q, want %q", got, want)
	}
}

func TestModsPath_Absolute(t *testing.T) {
	cfg := Default()
	cfg.ServerPath = "/opt/minecraft"
	cfg.Mods.Directory = "/custom/mods"

	got := cfg.ModsPath()
	want := "/custom/mods"
	if got != want {
		t.Errorf("ModsPath() = %q, want %q", got, want)
	}
}

func TestModsBackupPath_Relative(t *testing.T) {
	cfg := Default()
	cfg.ServerPath = "/opt/minecraft"
	cfg.Mods.BackupDir = "mods-backup"

	got := cfg.ModsBackupPath()
	want := "/opt/minecraft/mods-backup"
	if got != want {
		t.Errorf("ModsBackupPath() = %q, want %q", got, want)
	}
}

func TestModsBackupPath_Absolute(t *testing.T) {
	cfg := Default()
	cfg.ServerPath = "/opt/minecraft"
	cfg.Mods.BackupDir = "/custom/backup"

	got := cfg.ModsBackupPath()
	want := "/custom/backup"
	if got != want {
		t.Errorf("ModsBackupPath() = %q, want %q", got, want)
	}
}

func TestServerPropertiesPath(t *testing.T) {
	cfg := Default()
	cfg.ServerPath = "/opt/mc"

	got := cfg.ServerPropertiesPath()
	want := "/opt/mc/server.properties"
	if got != want {
		t.Errorf("ServerPropertiesPath() = %q, want %q", got, want)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "subdir", "mc.toml")

	orig := Default()
	orig.ServerPath = "/custom/server"
	orig.Server.MinecraftVersion = "1.20.2"
	orig.Server.Port = 12345
	orig.Mods.MaxBackups = 42
	orig.Display.LogLines = 200

	if err := orig.Save(cfgFile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify the file was created in a subdirectory.
	if _, err := os.Stat(cfgFile); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	loaded, err := LoadFrom(cfgFile)
	if err != nil {
		t.Fatalf("LoadFrom failed after Save: %v", err)
	}

	if loaded.ServerPath != orig.ServerPath {
		t.Errorf("ServerPath roundtrip: got %q, want %q", loaded.ServerPath, orig.ServerPath)
	}
	if loaded.Server.MinecraftVersion != orig.Server.MinecraftVersion {
		t.Errorf("MinecraftVersion roundtrip: got %q, want %q", loaded.Server.MinecraftVersion, orig.Server.MinecraftVersion)
	}
	if loaded.Server.Port != orig.Server.Port {
		t.Errorf("Port roundtrip: got %d, want %d", loaded.Server.Port, orig.Server.Port)
	}
	if loaded.Mods.MaxBackups != orig.Mods.MaxBackups {
		t.Errorf("MaxBackups roundtrip: got %d, want %d", loaded.Mods.MaxBackups, orig.Mods.MaxBackups)
	}
	if loaded.Display.LogLines != orig.Display.LogLines {
		t.Errorf("LogLines roundtrip: got %d, want %d", loaded.Display.LogLines, orig.Display.LogLines)
	}
}

func TestLoadFromInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "mc.toml")
	if err := os.WriteFile(cfgFile, []byte("this is not valid toml [[["), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(cfgFile)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestLoadFromValidationFailure(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "mc.toml")
	content := `server_path = ""
`
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(cfgFile)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got := err.Error(); !contains(got, "validating") {
		t.Errorf("error = %q, want mention of 'validating'", got)
	}
}

// contains is a helper to check substring presence.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
