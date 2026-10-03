package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config holds all configuration for the mc server manager.
type Config struct {
	ServerPath string         `toml:"server_path"`
	Server     ServerConfig   `toml:"server"`
	Systemd    SystemdConfig  `toml:"systemd"`
	Console    ConsoleConfig  `toml:"console"`
	Modrinth   ModrinthConfig `toml:"modrinth"`
	Mods       ModsConfig     `toml:"mods"`
	Backup     BackupConfig   `toml:"backup"`
	Display    DisplayConfig  `toml:"display"`
}

// ServerConfig holds Minecraft server settings.
type ServerConfig struct {
	MinecraftVersion string   `toml:"minecraft_version"`
	Loader           string   `toml:"loader"`
	LoaderVersion    string   `toml:"loader_version"`
	Java             string   `toml:"java"`
	MinMemory        string   `toml:"min_memory"`
	MaxMemory        string   `toml:"max_memory"`
	JVMArgs          []string `toml:"jvm_args"`
	Jar              string   `toml:"jar"`
	Port             int      `toml:"port"`
}

// SystemdConfig holds systemd service settings.
type SystemdConfig struct {
	Unit          string `toml:"unit"`
	StopTimeout   int    `toml:"stop_timeout"`
	RestartPolicy string `toml:"restart_policy"`
}

// ConsoleConfig holds RCON / console connection settings.
// The RCON password is intentionally NOT stored here; it is read from
// server.properties at runtime.
type ConsoleConfig struct {
	Method   string `toml:"method"`
	RCONPort int    `toml:"rcon_port"`
	RCONHost string `toml:"rcon_host"`
}

// ModrinthConfig holds Modrinth API settings.
type ModrinthConfig struct {
	Enabled   bool   `toml:"enabled"`
	UserAgent string `toml:"user_agent"`
	Timeout   int    `toml:"timeout"`
}

// ModsConfig holds mod management settings.
type ModsConfig struct {
	Directory  string            `toml:"directory"`
	BackupDir  string            `toml:"backup_dir"`
	MaxBackups int               `toml:"max_backups"`
	Pinned     map[string]string `toml:"pinned"`
}

// BackupConfig holds world backup settings.
type BackupConfig struct {
	Directory string `toml:"directory"`
}

// DisplayConfig holds terminal display settings.
type DisplayConfig struct {
	Color    string `toml:"color"`
	LogLines int    `toml:"log_lines"`
}

// Default returns a Config populated with sensible default values.
func Default() *Config {
	return &Config{
		ServerPath: "/home/ubuntu/minecraft",
		Server: ServerConfig{
			MinecraftVersion: "1.21.1",
			Loader:           "fabric",
			Java:             "/usr/bin/java",
			MinMemory:        "2G",
			MaxMemory:        "8G",
			Jar:              "server.jar",
			Port:             25565,
		},
		Systemd: SystemdConfig{
			Unit:          "minecraft.service",
			StopTimeout:   60,
			RestartPolicy: "on-failure",
		},
		Console: ConsoleConfig{
			Method:   "rcon",
			RCONPort: 25575,
			RCONHost: "127.0.0.1",
		},
		Modrinth: ModrinthConfig{
			Enabled:   true,
			UserAgent: "mc-server-manager/1.0 (github.com/mc-server-manager)",
			Timeout:   30,
		},
		Mods: ModsConfig{
			Directory:  "mods",
			BackupDir:  "mods-backup",
			MaxBackups: 5,
			Pinned:     make(map[string]string),
		},
		Backup: BackupConfig{},
		Display: DisplayConfig{
			Color:    "auto",
			LogLines: 50,
		},
	}
}

// configSearchPaths returns the ordered list of paths to check for a config file.
func configSearchPaths() []string {
	var paths []string

	// 1. $MC_CONFIG environment variable
	if env := os.Getenv("MC_CONFIG"); env != "" {
		paths = append(paths, env)
	}

	// 2. ./mc.toml (current directory)
	paths = append(paths, "mc.toml")

	// 3. ~/.config/mc/config.toml
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "mc", "config.toml"))
	}

	// 4. /etc/mc/config.toml
	paths = append(paths, "/etc/mc/config.toml")

	return paths
}

// FindConfigPath returns the path of the first config file found in the search
// order, or an empty string if none is found.
func FindConfigPath() string {
	for _, p := range configSearchPaths() {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Load finds and loads the first available config file. If no config file is
// found, it returns the default configuration.
func Load() (*Config, error) {
	path := FindConfigPath()
	if path == "" {
		return Default(), nil
	}
	return LoadFrom(path)
}

// LoadFrom loads configuration from a specific file path. Values not present in
// the file retain their defaults.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	cfg := Default()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config %s: %w", path, err)
	}

	return cfg, nil
}

// Validate checks the configuration for obvious errors.
func (c *Config) Validate() error {
	var errs []string

	if c.ServerPath == "" {
		errs = append(errs, "server_path must not be empty")
	}
	if c.Server.Jar == "" {
		errs = append(errs, "server.jar must not be empty")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errs = append(errs, "server.port must be between 1 and 65535")
	}
	if c.Systemd.Unit == "" {
		errs = append(errs, "systemd.unit must not be empty")
	}
	if c.Systemd.StopTimeout < 0 {
		errs = append(errs, "systemd.stop_timeout must not be negative")
	}
	if c.Console.Method != "" && c.Console.Method != "rcon" {
		errs = append(errs, "console.method must be \"rcon\" or empty")
	}
	if c.Console.RCONPort < 1 || c.Console.RCONPort > 65535 {
		errs = append(errs, "console.rcon_port must be between 1 and 65535")
	}
	if c.Modrinth.Timeout < 0 {
		errs = append(errs, "modrinth.timeout must not be negative")
	}
	if c.Mods.MaxBackups < 0 {
		errs = append(errs, "mods.max_backups must not be negative")
	}
	if c.Display.Color != "" && c.Display.Color != "auto" &&
		c.Display.Color != "always" && c.Display.Color != "never" {
		errs = append(errs, "display.color must be \"auto\", \"always\", or \"never\"")
	}
	if c.Display.LogLines < 0 {
		errs = append(errs, "display.log_lines must not be negative")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Save writes the configuration to a TOML file at the given path, creating
// parent directories as needed.
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory %s: %w", dir, err)
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config to %s: %w", path, err)
	}
	return nil
}

// ServerPropertiesPath returns the absolute path to server.properties.
func (c *Config) ServerPropertiesPath() string {
	return filepath.Join(c.ServerPath, "server.properties")
}

// ModsPath returns the absolute path to the mods directory.
func (c *Config) ModsPath() string {
	if filepath.IsAbs(c.Mods.Directory) {
		return c.Mods.Directory
	}
	return filepath.Join(c.ServerPath, c.Mods.Directory)
}

// ModsBackupPath returns the absolute path to the mods backup directory.
func (c *Config) ModsBackupPath() string {
	if filepath.IsAbs(c.Mods.BackupDir) {
		return c.Mods.BackupDir
	}
	return filepath.Join(c.ServerPath, c.Mods.BackupDir)
}

// ReadRCONPassword reads the rcon.password value from server.properties.
// It returns an error if the file cannot be read or the key is not found.
func (c *Config) ReadRCONPassword() (string, error) {
	path := c.ServerPropertiesPath()

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening server.properties: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and blank lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		if strings.TrimSpace(key) == "rcon.password" {
			pw := strings.TrimSpace(value)
			if pw == "" {
				return "", fmt.Errorf("rcon.password is empty in %s", path)
			}
			return pw, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading server.properties: %w", err)
	}

	return "", fmt.Errorf("rcon.password not found in %s", path)
}

// DefaultJVMArgs returns the Aikar's optimized JVM flags for Minecraft.
func DefaultJVMArgs() []string {
	return []string{
		"-XX:+UseG1GC",
		"-XX:+ParallelRefProcEnabled",
		"-XX:MaxGCPauseMillis=200",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:+DisableExplicitGC",
		"-XX:+AlwaysPreTouch",
		"-XX:G1NewSizePercent=30",
		"-XX:G1MaxNewSizePercent=40",
		"-XX:G1HeapRegionSize=8M",
		"-XX:G1ReservePercent=20",
		"-XX:G1MixedGCCountTarget=4",
		"-XX:InitiatingHeapOccupancyPercent=15",
		"-XX:G1MixedGCLiveThresholdPercent=90",
		"-XX:G1RSetUpdatingPauseTimePercent=5",
		"-XX:SurvivorRatio=32",
		"-XX:+PerfDisableSharedMem",
		"-XX:MaxTenuringThreshold=1",
		"-Dusing.aikars.flags=https://mcflags.emc.gs",
		"-Daikars.new.flags=true",
	}
}

// JVMArgsList returns the full JVM arguments list: memory flags + custom or
// default Aikar flags.
func (c *Config) JVMArgsList() []string {
	args := []string{
		"-Xms" + c.Server.MinMemory,
		"-Xmx" + c.Server.MaxMemory,
	}
	if len(c.Server.JVMArgs) > 0 {
		args = append(args, c.Server.JVMArgs...)
	} else {
		args = append(args, DefaultJVMArgs()...)
	}
	return args
}

// GenerateEnvironmentFile writes the systemd EnvironmentFile that overrides the
// default values baked into the service unit. Each variable is a single token
// (no spaces) - systemd expands ${VAR} in ExecStart argument positions only.
//
//	MC_MIN_MEMORY=-Xms6G   → one JVM flag
//	MC_MAX_MEMORY=-Xmx10G  → one JVM flag
//	MC_JAR=server.jar      → jar filename
//
// Note: MC_JAVA is intentionally absent. systemd does not expand ${VAR} for
// the executable path, so the java binary is hardcoded in the service unit as
// /usr/bin/java (managed via update-alternatives on Ubuntu).
//
// Regenerate after changing memory or jar in config.toml.
func (c *Config) GenerateEnvironmentFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	var b strings.Builder
	b.WriteString("# Generated by mc-server-manager - do not edit manually.\n")
	b.WriteString("# Regenerate with: sudo mc service generate-env\n\n")
	fmt.Fprintf(&b, "MC_JAR=%s\n", c.Server.Jar)
	fmt.Fprintf(&b, "MC_MIN_MEMORY=-Xms%s\n", c.Server.MinMemory)
	fmt.Fprintf(&b, "MC_MAX_MEMORY=-Xmx%s\n", c.Server.MaxMemory)

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("writing environment file %s: %w", path, err)
	}
	return nil
}

// DefaultEnvFilePath returns the standard location for the environment file.
func DefaultEnvFilePath() string {
	return "/etc/mc/minecraft.env"
}
