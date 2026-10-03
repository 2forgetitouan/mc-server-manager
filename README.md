# mc -- Minecraft Server Manager

A modern CLI tool to manage a single Minecraft Fabric server on Linux with
systemd. No Docker, no tmux, no screen, no web panel -- just a clean binary.

## Features

- **systemd-native** server lifecycle (start / stop / restart)
- **Real-time status** with CPU, memory, uptime, and player count
- **Log streaming** via journald with colour-coded output
- **Interactive console** powered by RCON, with command history
- **Mod updates** from the Modrinth API (hash-based identification, integrity
  verification, automatic backup and rollback)
- **Diagnostics** (`mc doctor`) to verify your installation
- **TOML configuration** with sensible defaults
- **ARM64 and AMD64** support

## Requirements

| Dependency | Notes |
|---|---|
| Linux | Ubuntu recommended; any systemd-based distro works |
| systemd | Used for service management and log streaming |
| Go 1.22+ | For building from source |
| Java 17+ | OpenJDK recommended |
| Minecraft server | Fabric, Forge, or Vanilla |

## Quick Start

```bash
git clone https://github.com/your-user/mc-server-manager.git
cd mc-server-manager
go build -o mc ./cmd/mc
sudo cp mc /usr/local/bin/

mc config init          # creates ~/.config/mc/config.toml
# Edit ~/.config/mc/config.toml to match your server
mc doctor               # verify everything is wired up
mc start                # start the server
```

## Installation

### Build from source

```bash
git clone https://github.com/your-user/mc-server-manager.git
cd mc-server-manager
go build -o mc ./cmd/mc
```

Copy the binary somewhere on your `$PATH`:

```bash
sudo cp mc /usr/local/bin/
```

### Cross-compile

Build for a different architecture without leaving your workstation:

```bash
# ARM64 (Raspberry Pi, Oracle Cloud Ampere, etc.)
GOOS=linux GOARCH=arm64 go build -o mc-linux-arm64 ./cmd/mc

# AMD64
GOOS=linux GOARCH=amd64 go build -o mc-linux-amd64 ./cmd/mc
```

Then copy the resulting binary to the target machine.

### Set the version at build time

```bash
go build -ldflags "-X main.version=1.2.3" -o mc ./cmd/mc
```

## Configuration

`mc` reads its configuration from the first file it finds in this order:

1. Path in the `$MC_CONFIG` environment variable
2. `./mc.toml` (current directory)
3. `~/.config/mc/config.toml`
4. `/etc/mc/config.toml`

Create a default config with:

```bash
mc config init
# or specify a custom path:
mc config init --path /etc/mc/config.toml
```

### Config file format

```toml
# Root directory of the Minecraft server installation.
server_path = "/home/ubuntu/minecraft"

# ---- Server settings ----
[server]
minecraft_version = "1.21.1"
loader            = "fabric"          # fabric, forge, or vanilla
# loader_version  = ""                # leave empty for latest
java              = "/usr/bin/java"
min_memory        = "2G"
max_memory        = "8G"
jar               = "server.jar"
port              = 25565
# jvm_args = ["-XX:+UseG1GC", "-XX:+ParallelRefProcEnabled"]

# ---- Systemd service ----
[systemd]
unit           = "minecraft.service"
stop_timeout   = 60                   # seconds for graceful stop
restart_policy = "on-failure"

# ---- Console / RCON ----
[console]
method    = "rcon"
rcon_port = 25575
rcon_host = "127.0.0.1"

# ---- Modrinth API ----
[modrinth]
enabled    = true
user_agent = "mc-server-manager/1.0 (github.com/mc-server-manager)"
timeout    = 30                       # HTTP timeout in seconds

# ---- Mod management ----
[mods]
directory   = "mods"                  # relative to server_path
backup_dir  = "mods-backup"           # relative to server_path
max_backups = 5

# Pin specific mods to a version so they are never updated.
# [mods.pinned]
# sodium  = "0.5.3"
# lithium = "0.11.2"

# ---- Display / terminal ----
[display]
color     = "auto"                    # auto | always | never
log_lines = 50                        # default number of lines for `mc logs`
```

### RCON password

The RCON password is **not** stored in the mc config file. It is read
automatically from your server's `server.properties` (`rcon.password=...`).
Make sure `enable-rcon=true` and `rcon.port` matches the value in `[console]`.

## Commands

### mc start

Start the Minecraft server via systemd.

```bash
mc start
```

### mc stop

Gracefully stop the server. Waits up to `stop_timeout` seconds.

```bash
mc stop
```

### mc restart

Restart the server (stop then start).

```bash
mc restart
```

### mc status

Show a live overview: service state, PID, uptime, CPU, memory, disk, and online
players.

```bash
mc status
```

### mc logs

Stream (or dump) server logs from journald. Lines containing `ERROR` or `WARN`
are colour-coded automatically.

```bash
mc logs                    # follow new log output (Ctrl+C to stop)
mc logs --no-follow        # print recent lines and exit
mc logs -n 100             # show the last 100 lines
mc logs --since 1h         # logs from the last hour
mc logs --since today      # logs since midnight
```

| Flag | Description |
|---|---|
| `-n`, `--lines` | Number of lines to show (default from config) |
| `--since` | Show logs since a time expression (e.g. `1h`, `today`, `2024-01-01`) |
| `--no-follow` | Print and exit instead of streaming |

### mc console

Open an interactive console to send commands to the running server via RCON.
Supports command history (arrow keys).

```bash
mc console
```

Type any Minecraft command (without the leading `/`). Press **Ctrl+C** or
**Ctrl+D** to exit the console.

The server must be running and RCON must be enabled in `server.properties`.

### mc update

Check for mod updates on Modrinth and optionally apply them.

```bash
mc update              # check and interactively apply updates
mc update --check      # only list available updates (no changes)
mc update --dry-run    # alias for --check
mc update --yes        # skip the confirmation prompt
```

| Flag | Description |
|---|---|
| `--check` | Only check for updates, do not apply |
| `--dry-run` | Alias for `--check` |
| `-y`, `--yes` | Skip the confirmation prompt |

### mc doctor

Run diagnostic checks against the installation: config validity, Java version,
server jar, systemd unit, RCON connectivity, and more.

```bash
mc doctor
```

Each check reports **PASS**, **WARN**, or **FAIL** with a suggested fix.

### mc config

Manage the configuration file.

```bash
mc config show           # display the active configuration
mc config init           # create a default config at ~/.config/mc/config.toml
mc config init -p /path  # create at a custom path
mc config path           # print the path of the active config file
```

### mc version

Print the build version.

```bash
mc version
```

### Global flags

| Flag | Description |
|---|---|
| `--color` | Colour output mode: `auto` (default), `always`, `never` |

## systemd Service

`mc` manages the server through a regular systemd unit. A minimal service file
looks like this:

```ini
[Unit]
Description=Minecraft Server
After=network.target

[Service]
Type=simple
User=minecraft
WorkingDirectory=/home/minecraft/server
ExecStart=/usr/bin/java -Xms2G -Xmx8G -jar server.jar --nogui
ExecStop=/usr/local/bin/mc stop
Restart=on-failure
RestartSec=10
TimeoutStopSec=60

# Security hardening
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=read-only
PrivateTmp=true
ReadWritePaths=/home/minecraft/server

[Install]
WantedBy=multi-user.target
```

Install and enable the unit:

```bash
sudo cp minecraft.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable minecraft.service
```

### Security hardening

The service file above includes several systemd directives that restrict the
server process:

- **NoNewPrivileges** -- the process cannot gain additional privileges.
- **ProtectSystem=full** -- `/usr` and `/boot` are mounted read-only.
- **ProtectHome=read-only** -- home directories are read-only except for the
  explicit `ReadWritePaths`.
- **PrivateTmp** -- the server gets its own `/tmp`.

## Mod Updates and Rollback

### How update detection works

1. `mc update` scans the `mods/` directory and computes the **SHA-512** hash of
   every `.jar` file.
2. The hashes are sent to the Modrinth API in a single bulk request to identify
   which mods are known.
3. For each identified mod, the tool fetches compatible versions filtered by your
   configured **Minecraft version** and **loader** (e.g. Fabric 1.21.1).
4. If a newer version exists whose hash differs from the installed file, it is
   reported as an available update.

Mods that are not on Modrinth (private or manual mods) are silently skipped.

### Applying updates

When you confirm (or pass `--yes`), each update goes through these steps:

1. The current `.jar` is backed up to `mods-backup/` with a timestamp.
2. The new file is downloaded to a temporary path.
3. The download is verified against the expected SHA-512 hash.
4. The file is atomically moved into place.
5. Old backups beyond `max_backups` are pruned.

An exclusive file lock prevents concurrent update operations.

### Rollback

If an update causes problems, the previous version of each mod is preserved in
the `mods-backup/` directory (configurable via `[mods] backup_dir`). Backups are
named `<mod>_YYYYMMDD_HHMMSS.jar` and the most recent `max_backups` copies are
kept.

### Pinning

To prevent a specific mod from being updated, add it to the `[mods.pinned]`
section by slug or project ID:

```toml
[mods.pinned]
sodium  = "0.5.3"
lithium = "0.11.2"
```

Pinned mods are skipped during update checks.

## Security

- **RCON bound to localhost** -- the default `rcon_host` is `127.0.0.1`; the
  RCON port is never exposed to the network.
- **RCON password never logged or displayed** -- the password is read from
  `server.properties` at runtime and kept in memory only.
- **systemd security hardening** -- the recommended service file uses
  `NoNewPrivileges`, `ProtectSystem`, `ProtectHome`, and `PrivateTmp`.
- **No shell injection** -- all external commands are executed with explicit
  argument lists, never through a shell.
- **File lock** -- concurrent mod update operations are prevented by an
  exclusive file-system lock (`flock`).

## Development

### Run tests

```bash
go test -race -v ./...
```

### Lint

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
staticcheck ./...
go vet ./...
```

### Build

```bash
go build -o mc ./cmd/mc
```

### Cross-compilation

```bash
# ARM64
GOOS=linux GOARCH=arm64 go build -o mc-linux-arm64 ./cmd/mc

# AMD64
GOOS=linux GOARCH=amd64 go build -o mc-linux-amd64 ./cmd/mc
```

### Project structure

```
cmd/mc/             Entry point
internal/
  cli/              Cobra command definitions
  config/           TOML config loading and defaults
  console/          Interactive RCON console (bubbletea)
  doctor/           Diagnostic checks
  modrinth/         Modrinth API client and types
  mods/             Mod scanning, updating, backup, rollback
  rcon/             RCON protocol implementation
  stats/            Process and system stats (CPU, memory, disk)
  systemd/          systemd/journalctl integration
  ui/               Terminal colours and table formatting
configs/            Example configuration files
systemd/            Example systemd unit files
```

## Uninstallation

1. Stop the server:

   ```bash
   mc stop
   ```

2. Remove the binary:

   ```bash
   sudo rm /usr/local/bin/mc
   ```

3. Remove the configuration (optional):

   ```bash
   rm -r ~/.config/mc
   ```

4. If you installed a systemd unit, disable and remove it:

   ```bash
   sudo systemctl disable minecraft.service
   sudo rm /etc/systemd/system/minecraft.service
   sudo systemctl daemon-reload
   ```

## Troubleshooting

### "failed to load config"

The tool could not find a configuration file. Run `mc config init` to create
one, then edit it to match your server installation.

### "server is not running" when using mc console

The RCON console requires a running server. Start it first with `mc start`.

### "failed to read RCON password"

Make sure `enable-rcon=true` and `rcon.password=<something>` are set in your
server's `server.properties`. The `rcon.port` must also match the `rcon_port`
value in your mc config.

### mc doctor reports FAIL for Java

Verify Java is installed and the `java` path in your config points to a valid
Java 17+ binary:

```bash
/usr/bin/java -version
```

### Logs show nothing / mc logs hangs

Confirm the systemd unit name in your config matches the actual service:

```bash
systemctl list-units --type=service | grep mine
```

### Mod update says "Modrinth integration is disabled"

Set `enabled = true` under `[modrinth]` in your config. This is expected for
Vanilla servers that do not use mods.

### Permission denied errors

The user running `mc` must have permission to control the systemd service. You
can either run as the same user the service runs under, or grant `systemctl`
permissions via polkit / sudoers.

## License

MIT
