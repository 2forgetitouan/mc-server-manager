package mods

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"mc-server-manager/internal/config"
	"mc-server-manager/internal/modrinth"
)

// Manager handles mod file operations: scanning, updating, backup, and rollback.
type Manager struct {
	cfg    *config.Config
	client *modrinth.Client
	mu     sync.Mutex
}

// ModFile represents a .jar mod file on disk.
type ModFile struct {
	Path string
	Name string
	Hash string // SHA-512 hex-encoded
	Size int64
}

// UpdateResult records the outcome of a single mod update attempt.
type UpdateResult struct {
	ModName      string
	ProjectTitle string
	OldFile      string
	NewFile      string
	OldVersion   string
	NewVersion   string
	Success      bool
	Error        error
	BackupPath   string
}

// NewManager creates a Manager backed by the given configuration.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:    cfg,
		client: modrinth.NewClient(cfg.Modrinth.UserAgent, time.Duration(cfg.Modrinth.Timeout)*time.Second),
	}
}

// ScanMods reads the mods directory and returns all .jar files with their
// SHA-512 hashes.
func (m *Manager) ScanMods() ([]ModFile, error) {
	modsDir := m.cfg.ModsPath()

	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return nil, fmt.Errorf("reading mods directory %s: %w", modsDir, err)
	}

	var mods []ModFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			continue
		}

		path := filepath.Join(modsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", entry.Name(), err)
		}

		hash, err := m.hashFile(path)
		if err != nil {
			return nil, fmt.Errorf("hashing %s: %w", entry.Name(), err)
		}

		mods = append(mods, ModFile{
			Path: path,
			Name: entry.Name(),
			Hash: hash,
			Size: info.Size(),
		})
	}

	return mods, nil
}

// CheckUpdates identifies available updates for mods recognised by Modrinth.
// Mods that are not on Modrinth are silently skipped (they are manual mods).
// Pinned mods (by project ID or slug) are also skipped.
func (m *Manager) CheckUpdates(ctx context.Context) ([]modrinth.UpdateInfo, error) {
	mods, err := m.ScanMods()
	if err != nil {
		return nil, err
	}
	if len(mods) == 0 {
		return nil, nil
	}

	// Build hash -> ModFile lookup.
	hashToMod := make(map[string]ModFile, len(mods))
	hashes := make([]string, 0, len(mods))
	for _, mod := range mods {
		hashToMod[mod.Hash] = mod
		hashes = append(hashes, mod.Hash)
	}

	// Bulk-identify installed mods by their SHA-512 hashes.
	identified, err := m.client.VersionsFromHashes(ctx, hashes, "sha512")
	if err != nil {
		return nil, fmt.Errorf("identifying mods via hash lookup: %w", err)
	}

	// Deduplicate by project ID so we only check each project once.
	type currentMod struct {
		version modrinth.Version
		file    ModFile
		hash    string
	}
	byProject := make(map[string]currentMod)
	for hash, ver := range identified {
		mod, ok := hashToMod[hash]
		if !ok {
			continue
		}
		if _, exists := byProject[ver.ProjectID]; exists {
			continue
		}
		byProject[ver.ProjectID] = currentMod{version: ver, file: mod, hash: hash}
	}

	var updates []modrinth.UpdateInfo

	for projectID, cur := range byProject {
		// Respect context cancellation between API calls.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Skip if pinned by project ID.
		if _, pinned := m.cfg.Mods.Pinned[projectID]; pinned {
			continue
		}

		// Fetch project metadata for server-side check and title.
		project, err := m.client.GetProject(ctx, projectID)
		if err != nil {
			// Cannot look up the project; skip silently.
			continue
		}

		// Skip if pinned by slug.
		if _, pinned := m.cfg.Mods.Pinned[project.Slug]; pinned {
			continue
		}

		// Only consider mods that work on the server side.
		if !m.isServerSideMod(project) {
			continue
		}

		// Fetch versions compatible with our Minecraft version and loader.
		versions, err := m.client.GetVersions(ctx, projectID,
			[]string{m.cfg.Server.MinecraftVersion},
			[]string{m.cfg.Server.Loader})
		if err != nil {
			continue
		}

		latest := findLatestVersion(versions)
		if latest == nil {
			continue
		}

		file := primaryFile(latest)
		if file == nil {
			continue
		}

		// Already on the latest version; nothing to do.
		if file.Hashes.SHA512 == cur.hash {
			continue
		}

		updates = append(updates, modrinth.UpdateInfo{
			ProjectSlug:   project.Slug,
			ProjectTitle:  project.Title,
			CurrentFile:   cur.file.Name,
			CurrentHash:   cur.hash,
			LatestVersion: *latest,
			LatestFile:    *file,
			Dependencies:  latest.Dependencies,
		})
	}

	return updates, nil
}

// ApplyUpdates downloads and installs each update, backing up the current file
// first.  It acquires an exclusive lock for the duration of the operation.
func (m *Manager) ApplyUpdates(ctx context.Context, updates []modrinth.UpdateInfo) ([]UpdateResult, error) {
	unlock, err := m.LockFile()
	if err != nil {
		return nil, fmt.Errorf("acquiring update lock: %w", err)
	}
	defer unlock()

	modsDir := m.cfg.ModsPath()
	results := make([]UpdateResult, 0, len(updates))

	for _, upd := range updates {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		res := UpdateResult{
			ModName:      upd.CurrentFile,
			ProjectTitle: upd.ProjectTitle,
			OldFile:      upd.CurrentFile,
			NewFile:      upd.LatestFile.Filename,
			OldVersion:   upd.CurrentHash,
			NewVersion:   upd.LatestVersion.VersionNumber,
		}

		// 1. Backup the current mod file.
		currentPath := filepath.Join(modsDir, upd.CurrentFile)
		backupPath, bErr := m.backupMod(currentPath)
		if bErr != nil {
			res.Error = fmt.Errorf("backup failed: %w", bErr)
			results = append(results, res)
			continue
		}
		res.BackupPath = backupPath

		// 2. Download the new version to a temporary file.
		tmpPath := filepath.Join(modsDir, upd.LatestFile.Filename+".tmp")
		if dErr := m.client.DownloadFile(ctx, upd.LatestFile.URL, tmpPath); dErr != nil {
			res.Error = fmt.Errorf("download failed: %w", dErr)
			results = append(results, res)
			continue
		}

		// 3. Verify the downloaded file's hash.
		if vErr := m.verifyDownload(tmpPath, upd.LatestFile.Hashes); vErr != nil {
			os.Remove(tmpPath)
			res.Error = fmt.Errorf("verification failed: %w", vErr)
			results = append(results, res)
			continue
		}

		// 4. Atomically move the temp file into place.
		newPath := filepath.Join(modsDir, upd.LatestFile.Filename)
		if rErr := m.atomicReplace(tmpPath, newPath); rErr != nil {
			os.Remove(tmpPath)
			res.Error = fmt.Errorf("replace failed: %w", rErr)
			results = append(results, res)
			continue
		}

		// 5. If the filename changed, remove the old file.
		if upd.LatestFile.Filename != upd.CurrentFile {
			os.Remove(currentPath) // best-effort
		}

		// 6. Prune old backups beyond the configured maximum.
		baseName := strings.TrimSuffix(upd.CurrentFile, ".jar")
		_ = m.cleanOldBackups(baseName)

		res.Success = true
		results = append(results, res)
	}

	return results, nil
}

// Rollback restores the most recent backup for the given mod file name.
// modFileName should be the .jar filename (e.g. "sodium-0.5.3.jar").
func (m *Manager) Rollback(modFileName string) error {
	backupDir := m.cfg.ModsBackupPath()
	modsDir := m.cfg.ModsPath()

	baseName := strings.TrimSuffix(modFileName, ".jar")
	prefix := baseName + "_"

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("reading backup directory: %w", err)
	}

	var backups []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".jar") {
			backups = append(backups, name)
		}
	}

	if len(backups) == 0 {
		return fmt.Errorf("no backups found for %s", modFileName)
	}

	// Timestamps in the filename make lexicographic order chronological.
	sort.Strings(backups)
	latestBackup := backups[len(backups)-1]

	backupPath := filepath.Join(backupDir, latestBackup)
	destPath := filepath.Join(modsDir, modFileName)

	src, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("opening backup %s: %w", latestBackup, err)
	}
	defer src.Close()

	tmpPath := destPath + ".tmp"
	dst, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("copying backup: %w", err)
	}
	if err := dst.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("flushing temp file: %w", err)
	}

	return m.atomicReplace(tmpPath, destPath)
}

// ListBackups returns the names of all backup .jar files, sorted
// chronologically (oldest first).
func (m *Manager) ListBackups() ([]string, error) {
	backupDir := m.cfg.ModsBackupPath()

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading backup directory: %w", err)
	}

	var backups []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jar") {
			backups = append(backups, entry.Name())
		}
	}

	sort.Strings(backups)
	return backups, nil
}

// LockFile acquires an exclusive file-system lock to prevent concurrent update
// operations (both within the same process and across processes).  The returned
// function releases the lock and must always be called when the caller is done.
func (m *Manager) LockFile() (func(), error) {
	m.mu.Lock()

	modsDir := m.cfg.ModsPath()
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("ensuring mods directory exists: %w", err)
	}

	lockPath := filepath.Join(modsDir, ".update.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("creating lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		m.mu.Unlock()
		return nil, fmt.Errorf("acquiring file lock (another update in progress?): %w", err)
	}

	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		os.Remove(lockPath)
		m.mu.Unlock()
	}, nil
}

// ---------- internal helpers ----------

// hashFile computes the hex-encoded SHA-512 digest of the file at path.
func (m *Manager) hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// backupMod copies the mod at modPath into the backup directory with a
// timestamped name: <basename>_YYYYMMDD_HHMMSS.jar.
func (m *Manager) backupMod(modPath string) (string, error) {
	backupDir := m.cfg.ModsBackupPath()
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("creating backup directory: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(modPath), ".jar")
	ts := time.Now().Format("20060102_150405")
	backupName := fmt.Sprintf("%s_%s.jar", baseName, ts)
	backupPath := filepath.Join(backupDir, backupName)

	src, err := os.Open(modPath)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", filepath.Base(modPath), err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("creating backup file: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(backupPath)
		return "", fmt.Errorf("writing backup: %w", err)
	}
	if err := dst.Close(); err != nil {
		os.Remove(backupPath)
		return "", fmt.Errorf("flushing backup: %w", err)
	}

	return backupPath, nil
}

// atomicReplace moves src to dst via rename, which is atomic when both reside
// on the same filesystem.
func (m *Manager) atomicReplace(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", filepath.Base(src), filepath.Base(dst), err)
	}
	return nil
}

// cleanOldBackups keeps only the most recent MaxBackups files for the given
// mod name prefix.
func (m *Manager) cleanOldBackups(modName string) error {
	backupDir := m.cfg.ModsBackupPath()

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return err
	}

	prefix := modName + "_"
	var backups []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".jar") {
			backups = append(backups, name)
		}
	}

	maxBackups := m.cfg.Mods.MaxBackups
	if maxBackups <= 0 || len(backups) <= maxBackups {
		return nil
	}

	// Timestamps in filename keep lexicographic order chronological.
	sort.Strings(backups)

	// Remove the oldest entries.
	for _, name := range backups[:len(backups)-maxBackups] {
		os.Remove(filepath.Join(backupDir, name))
	}

	return nil
}

// verifyDownload computes the SHA-512 hash of the file at path and compares it
// with the expected hashes.
func (m *Manager) verifyDownload(path string, expected modrinth.FileHashes) error {
	hash, err := m.hashFile(path)
	if err != nil {
		return fmt.Errorf("hashing downloaded file: %w", err)
	}

	if expected.SHA512 != "" && hash != expected.SHA512 {
		return fmt.Errorf("SHA-512 mismatch: expected %s, got %s", expected.SHA512, hash)
	}

	return nil
}

// isServerSideMod returns true unless the project explicitly declares it does
// not support the server side.
func (m *Manager) isServerSideMod(project *modrinth.Project) bool {
	return project.ServerSide != "unsupported"
}

// ---------- version selection helpers ----------

// findLatestVersion picks the best candidate from a list of compatible
// versions.  It prefers "release" type; falls back to "beta" if no release
// exists.  Versions are assumed to be ordered newest-first (the Modrinth API
// default).
func findLatestVersion(versions []modrinth.Version) *modrinth.Version {
	var bestRelease, bestBeta *modrinth.Version
	for i := range versions {
		v := &versions[i]
		switch v.VersionType {
		case "release":
			if bestRelease == nil {
				bestRelease = v
			}
		case "beta":
			if bestBeta == nil {
				bestBeta = v
			}
		}
	}
	if bestRelease != nil {
		return bestRelease
	}
	return bestBeta
}

// primaryFile returns the primary file of a version, falling back to the first
// file if none is marked primary.
func primaryFile(v *modrinth.Version) *modrinth.VersionFile {
	if len(v.Files) == 0 {
		return nil
	}
	for i := range v.Files {
		if v.Files[i].Primary {
			return &v.Files[i]
		}
	}
	return &v.Files[0]
}
