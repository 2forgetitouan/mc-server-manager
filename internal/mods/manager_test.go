package mods

import (
	"crypto/sha512"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"mc-server-manager/internal/config"
	"mc-server-manager/internal/modrinth"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	modsDir := filepath.Join(dir, "mods")
	backupDir := filepath.Join(dir, "mods-backup")
	os.MkdirAll(modsDir, 0o755)
	os.MkdirAll(backupDir, 0o755)

	cfg := config.Default()
	cfg.ServerPath = dir
	cfg.Mods.Directory = "mods"
	cfg.Mods.BackupDir = "mods-backup"
	cfg.Mods.MaxBackups = 3
	return cfg
}

func createFakeJar(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("creating fake jar %s: %v", name, err)
	}
	return path
}

func sha512Hex(data string) string {
	h := sha512.Sum512([]byte(data))
	return hex.EncodeToString(h[:])
}

func TestScanMods(t *testing.T) {
	cfg := testConfig(t)
	modsDir := cfg.ModsPath()

	createFakeJar(t, modsDir, "sodium-0.5.jar", "sodium content")
	createFakeJar(t, modsDir, "lithium-0.12.jar", "lithium content")
	createFakeJar(t, modsDir, "readme.txt", "not a jar")

	// Also create a subdirectory that should be ignored.
	os.MkdirAll(filepath.Join(modsDir, "subdir"), 0o755)

	mgr := NewManager(cfg)
	mods, err := mgr.ScanMods()
	if err != nil {
		t.Fatalf("ScanMods failed: %v", err)
	}

	if len(mods) != 2 {
		t.Fatalf("ScanMods found %d mods, want 2", len(mods))
	}

	// Check that the mods have correct names and non-empty hashes.
	names := make(map[string]bool)
	for _, mod := range mods {
		names[mod.Name] = true
		if mod.Hash == "" {
			t.Errorf("mod %s has empty hash", mod.Name)
		}
		if mod.Size == 0 {
			t.Errorf("mod %s has zero size", mod.Name)
		}
		if mod.Path == "" {
			t.Errorf("mod %s has empty path", mod.Name)
		}
	}

	if !names["sodium-0.5.jar"] {
		t.Error("missing sodium-0.5.jar in scan results")
	}
	if !names["lithium-0.12.jar"] {
		t.Error("missing lithium-0.12.jar in scan results")
	}
}

func TestScanMods_EmptyDir(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	mods, err := mgr.ScanMods()
	if err != nil {
		t.Fatalf("ScanMods failed: %v", err)
	}
	if len(mods) != 0 {
		t.Errorf("ScanMods found %d mods in empty dir, want 0", len(mods))
	}
}

func TestScanMods_NonexistentDir(t *testing.T) {
	cfg := config.Default()
	cfg.ServerPath = "/no/such/directory"
	cfg.Mods.Directory = "mods"

	mgr := NewManager(cfg)
	_, err := mgr.ScanMods()
	if err == nil {
		t.Fatal("expected error for non-existent mods directory")
	}
}

func TestHashFile(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	dir := t.TempDir()
	content := "test file content for hashing"
	path := filepath.Join(dir, "test.jar")
	os.WriteFile(path, []byte(content), 0o644)

	hash, err := mgr.hashFile(path)
	if err != nil {
		t.Fatalf("hashFile failed: %v", err)
	}

	expected := sha512Hex(content)
	if hash != expected {
		t.Errorf("hash = %q, want %q", hash, expected)
	}
}

func TestHashFile_NonexistentFile(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	_, err := mgr.hashFile("/no/such/file.jar")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestBackupMod(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	modsDir := cfg.ModsPath()
	content := "original mod content"
	modPath := createFakeJar(t, modsDir, "test-mod.jar", content)

	backupPath, err := mgr.backupMod(modPath)
	if err != nil {
		t.Fatalf("backupMod failed: %v", err)
	}

	// Verify backup file exists.
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file does not exist: %v", err)
	}

	// Verify backup content matches original.
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(backupData) != content {
		t.Errorf("backup content = %q, want %q", string(backupData), content)
	}

	// Verify backup is in the backup directory.
	backupDir := cfg.ModsBackupPath()
	if filepath.Dir(backupPath) != backupDir {
		t.Errorf("backup dir = %q, want %q", filepath.Dir(backupPath), backupDir)
	}
}

func TestAtomicReplace(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	dir := t.TempDir()

	srcContent := "new content"
	srcPath := filepath.Join(dir, "src.tmp")
	os.WriteFile(srcPath, []byte(srcContent), 0o644)

	dstPath := filepath.Join(dir, "dst.jar")
	os.WriteFile(dstPath, []byte("old content"), 0o644)

	if err := mgr.atomicReplace(srcPath, dstPath); err != nil {
		t.Fatalf("atomicReplace failed: %v", err)
	}

	// src should no longer exist.
	if _, err := os.Stat(srcPath); err == nil {
		t.Error("src file should not exist after rename")
	}

	// dst should have the new content.
	data, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading dst: %v", err)
	}
	if string(data) != srcContent {
		t.Errorf("dst content = %q, want %q", string(data), srcContent)
	}
}

func TestVerifyDownload_Match(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	dir := t.TempDir()
	content := "verified content"
	path := filepath.Join(dir, "mod.jar")
	os.WriteFile(path, []byte(content), 0o644)

	expected := modrinth.FileHashes{
		SHA512: sha512Hex(content),
	}

	err := mgr.verifyDownload(path, expected)
	if err != nil {
		t.Fatalf("verifyDownload failed: %v", err)
	}
}

func TestVerifyDownload_Mismatch(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	dir := t.TempDir()
	content := "actual content"
	path := filepath.Join(dir, "mod.jar")
	os.WriteFile(path, []byte(content), 0o644)

	expected := modrinth.FileHashes{
		SHA512: sha512Hex("different content"),
	}

	err := mgr.verifyDownload(path, expected)
	if err == nil {
		t.Fatal("expected error for hash mismatch")
	}
}

func TestVerifyDownload_EmptyHash(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	dir := t.TempDir()
	path := filepath.Join(dir, "mod.jar")
	os.WriteFile(path, []byte("content"), 0o644)

	// Empty expected hash should pass (no hash to check against).
	expected := modrinth.FileHashes{SHA512: ""}
	err := mgr.verifyDownload(path, expected)
	if err != nil {
		t.Fatalf("verifyDownload should pass with empty expected hash: %v", err)
	}
}

func TestCleanOldBackups(t *testing.T) {
	cfg := testConfig(t)
	cfg.Mods.MaxBackups = 2
	mgr := NewManager(cfg)

	backupDir := cfg.ModsBackupPath()

	// Create 5 backup files with different timestamps.
	names := []string{
		"test-mod_20240101_100000.jar",
		"test-mod_20240102_100000.jar",
		"test-mod_20240103_100000.jar",
		"test-mod_20240104_100000.jar",
		"test-mod_20240105_100000.jar",
	}
	for _, name := range names {
		createFakeJar(t, backupDir, name, "backup content")
	}

	err := mgr.cleanOldBackups("test-mod")
	if err != nil {
		t.Fatalf("cleanOldBackups failed: %v", err)
	}

	// Only the 2 most recent should remain.
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("reading backup dir: %v", err)
	}

	var remaining []string
	for _, e := range entries {
		remaining = append(remaining, e.Name())
	}

	if len(remaining) != 2 {
		t.Fatalf("remaining = %d, want 2: %v", len(remaining), remaining)
	}

	// The two most recent should survive.
	for _, r := range remaining {
		if r != "test-mod_20240104_100000.jar" && r != "test-mod_20240105_100000.jar" {
			t.Errorf("unexpected surviving backup: %s", r)
		}
	}
}

func TestCleanOldBackups_UnderLimit(t *testing.T) {
	cfg := testConfig(t)
	cfg.Mods.MaxBackups = 10
	mgr := NewManager(cfg)

	backupDir := cfg.ModsBackupPath()
	createFakeJar(t, backupDir, "test-mod_20240101_100000.jar", "backup")

	err := mgr.cleanOldBackups("test-mod")
	if err != nil {
		t.Fatalf("cleanOldBackups failed: %v", err)
	}

	// Should not delete anything.
	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 1 {
		t.Errorf("entries = %d, want 1", len(entries))
	}
}

func TestRollback(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	modsDir := cfg.ModsPath()
	backupDir := cfg.ModsBackupPath()

	// Create a current mod file.
	createFakeJar(t, modsDir, "test-mod.jar", "current content")

	// Create a backup.
	backupContent := "original content from backup"
	createFakeJar(t, backupDir, "test-mod_20240101_100000.jar", backupContent)

	err := mgr.Rollback("test-mod.jar")
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify the mod file now has the backup content.
	data, err := os.ReadFile(filepath.Join(modsDir, "test-mod.jar"))
	if err != nil {
		t.Fatalf("reading restored mod: %v", err)
	}
	if string(data) != backupContent {
		t.Errorf("restored content = %q, want %q", string(data), backupContent)
	}
}

func TestRollback_NoBackups(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	err := mgr.Rollback("nonexistent-mod.jar")
	if err == nil {
		t.Fatal("expected error for rollback with no backups")
	}
}

func TestRollback_MultipleBackups(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	modsDir := cfg.ModsPath()
	backupDir := cfg.ModsBackupPath()

	createFakeJar(t, modsDir, "test-mod.jar", "current")

	// Create multiple backups; the latest one should be used.
	createFakeJar(t, backupDir, "test-mod_20240101_100000.jar", "old backup")
	createFakeJar(t, backupDir, "test-mod_20240102_100000.jar", "latest backup")

	err := mgr.Rollback("test-mod.jar")
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(modsDir, "test-mod.jar"))
	if string(data) != "latest backup" {
		t.Errorf("content = %q, want 'latest backup'", string(data))
	}
}

func TestLockFile(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	unlock, err := mgr.LockFile()
	if err != nil {
		t.Fatalf("LockFile failed: %v", err)
	}

	// Verify lock file exists.
	lockPath := filepath.Join(cfg.ModsPath(), ".update.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file does not exist: %v", err)
	}

	unlock()

	// After unlock, lock file should be removed.
	if _, err := os.Stat(lockPath); err == nil {
		t.Error("lock file should be removed after unlock")
	}
}

func TestLockFile_SecondAcquireFails(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	unlock, err := mgr.LockFile()
	if err != nil {
		t.Fatalf("first LockFile failed: %v", err)
	}
	defer unlock()

	// A second manager attempting to lock should fail.
	mgr2 := NewManager(cfg)
	_, err = mgr2.LockFile()
	if err == nil {
		t.Fatal("expected error acquiring second lock")
	}
}

func TestFindLatestVersion(t *testing.T) {
	tests := []struct {
		name     string
		versions []modrinth.Version
		wantID   string
		wantNil  bool
	}{
		{
			name:    "empty",
			wantNil: true,
		},
		{
			name: "single release",
			versions: []modrinth.Version{
				{ID: "v1", VersionType: "release"},
			},
			wantID: "v1",
		},
		{
			name: "release over beta",
			versions: []modrinth.Version{
				{ID: "v-beta", VersionType: "beta"},
				{ID: "v-release", VersionType: "release"},
			},
			wantID: "v-release",
		},
		{
			name: "beta fallback when no release",
			versions: []modrinth.Version{
				{ID: "v-beta1", VersionType: "beta"},
				{ID: "v-beta2", VersionType: "beta"},
			},
			wantID: "v-beta1",
		},
		{
			name: "first release wins",
			versions: []modrinth.Version{
				{ID: "v1", VersionType: "release"},
				{ID: "v2", VersionType: "release"},
			},
			wantID: "v1",
		},
		{
			name: "alpha ignored",
			versions: []modrinth.Version{
				{ID: "v-alpha", VersionType: "alpha"},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findLatestVersion(tt.versions)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil version")
			}
			if got.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestPrimaryFile(t *testing.T) {
	tests := []struct {
		name     string
		version  modrinth.Version
		wantName string
		wantNil  bool
	}{
		{
			name:    "no files",
			version: modrinth.Version{Files: nil},
			wantNil: true,
		},
		{
			name: "primary marked",
			version: modrinth.Version{Files: []modrinth.VersionFile{
				{Filename: "secondary.jar", Primary: false},
				{Filename: "primary.jar", Primary: true},
			}},
			wantName: "primary.jar",
		},
		{
			name: "no primary falls back to first",
			version: modrinth.Version{Files: []modrinth.VersionFile{
				{Filename: "first.jar", Primary: false},
				{Filename: "second.jar", Primary: false},
			}},
			wantName: "first.jar",
		},
		{
			name: "single file",
			version: modrinth.Version{Files: []modrinth.VersionFile{
				{Filename: "only.jar", Primary: false},
			}},
			wantName: "only.jar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := primaryFile(&tt.version)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil file")
			}
			if got.Filename != tt.wantName {
				t.Errorf("Filename = %q, want %q", got.Filename, tt.wantName)
			}
		})
	}
}

func TestListBackups(t *testing.T) {
	cfg := testConfig(t)
	mgr := NewManager(cfg)

	backupDir := cfg.ModsBackupPath()
	createFakeJar(t, backupDir, "mod-a_20240101_100000.jar", "a")
	createFakeJar(t, backupDir, "mod-b_20240102_100000.jar", "b")
	createFakeJar(t, backupDir, "readme.txt", "not a jar") // should be ignored

	backups, err := mgr.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) != 2 {
		t.Fatalf("len(backups) = %d, want 2", len(backups))
	}
}

func TestListBackups_NoDir(t *testing.T) {
	cfg := config.Default()
	cfg.ServerPath = "/no/such/path"
	cfg.Mods.BackupDir = "backups"

	mgr := NewManager(cfg)
	backups, err := mgr.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups should return nil for non-existent dir: %v", err)
	}
	if backups != nil {
		t.Errorf("expected nil, got %v", backups)
	}
}

func TestNewManager(t *testing.T) {
	cfg := config.Default()
	mgr := NewManager(cfg)
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	if mgr.cfg != cfg {
		t.Error("manager config doesn't match")
	}
	if mgr.client == nil {
		t.Error("manager client is nil")
	}
}
