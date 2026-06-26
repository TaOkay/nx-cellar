package archive

import (
   "os"
   "path/filepath"
   "strings"
   "testing"

   "go.uber.org/zap"
)

func testLogger() *zap.SugaredLogger {
   logger, _ := zap.NewDevelopment()
   return logger.Sugar()
}

func TestNewArchiveManager_Valid(t *testing.T) {
   am, err := NewArchiveManager("/tmp/archive", testLogger())
   if err != nil {
      t.Fatalf("expected no error, got %v", err)
   }
   if am.archivePath != "/tmp/archive" {
      t.Errorf("expected archivePath %q, got %q", "/tmp/archive", am.archivePath)
   }
}

func TestNewArchiveManager_EmptyPath(t *testing.T) {
   _, err := NewArchiveManager("", testLogger())
   if err == nil {
      t.Fatal("expected error for empty path")
   }
   if !strings.Contains(err.Error(), "must not be empty") {
      t.Errorf("unexpected error message: %v", err)
   }
}

func TestNewArchiveManager_RelativePath(t *testing.T) {
   _, err := NewArchiveManager("relative/path", testLogger())
   if err == nil {
      t.Fatal("expected error for relative path")
   }
   if !strings.Contains(err.Error(), "must be absolute") {
      t.Errorf("unexpected error message: %v", err)
   }
}

func TestNewArchiveManager_PathTooLong(t *testing.T) {
   longPath := "/" + strings.Repeat("a", 260)
   _, err := NewArchiveManager(longPath, testLogger())
   if err == nil {
      t.Fatal("expected error for path exceeding max length")
   }
   if !strings.Contains(err.Error(), "at most 260 characters") {
      t.Errorf("unexpected error message: %v", err)
   }
}

func TestNewArchiveManager_NilLogger(t *testing.T) {
   _, err := NewArchiveManager("/tmp/archive", nil)
   if err == nil {
      t.Fatal("expected error for nil logger")
   }
   if !strings.Contains(err.Error(), "logger must not be nil") {
      t.Errorf("unexpected error message: %v", err)
   }
}

func TestEnsureArchiveDir(t *testing.T) {
   dir := t.TempDir()
   archivePath := filepath.Join(dir, "deep", "nested", "archive")

   am, err := NewArchiveManager(archivePath, testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   if err := am.EnsureArchiveDir(); err != nil {
      t.Fatalf("EnsureArchiveDir failed: %v", err)
   }

   info, err := os.Stat(archivePath)
   if err != nil {
      t.Fatalf("archive directory does not exist: %v", err)
   }
   if !info.IsDir() {
      t.Error("expected a directory")
   }
}

func TestMoveToArchive_SimpleMove(t *testing.T) {
   dir := t.TempDir()
   archivePath := filepath.Join(dir, "archive")
   srcFile := filepath.Join(dir, "game.nsp")

   if err := os.WriteFile(srcFile, []byte("test content"), 0644); err != nil {
      t.Fatalf("failed to create source file: %v", err)
   }

   am, err := NewArchiveManager(archivePath, testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   destPath, err := am.MoveToArchive(srcFile)
   if err != nil {
      t.Fatalf("MoveToArchive failed: %v", err)
   }

   expected := filepath.Join(archivePath, "game.nsp")
   if destPath != expected {
      t.Errorf("expected dest %q, got %q", expected, destPath)
   }

   // Source should no longer exist.
   if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
      t.Error("source file should have been removed")
   }

   // Destination should exist with correct content.
   data, err := os.ReadFile(destPath)
   if err != nil {
      t.Fatalf("failed to read destination file: %v", err)
   }
   if string(data) != "test content" {
      t.Errorf("expected content %q, got %q", "test content", string(data))
   }
}

func TestMoveToArchive_ConflictResolution(t *testing.T) {
   dir := t.TempDir()
   archivePath := filepath.Join(dir, "archive")

   am, err := NewArchiveManager(archivePath, testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   // Create archive dir and an existing file in it.
   if err := am.EnsureArchiveDir(); err != nil {
      t.Fatalf("EnsureArchiveDir failed: %v", err)
   }
   if err := os.WriteFile(filepath.Join(archivePath, "game.nsp"), []byte("existing"), 0644); err != nil {
      t.Fatalf("failed to create existing file: %v", err)
   }

   // Create the source file.
   srcFile := filepath.Join(dir, "game.nsp")
   if err := os.WriteFile(srcFile, []byte("new content"), 0644); err != nil {
      t.Fatalf("failed to create source file: %v", err)
   }

   destPath, err := am.MoveToArchive(srcFile)
   if err != nil {
      t.Fatalf("MoveToArchive failed: %v", err)
   }

   expected := filepath.Join(archivePath, "game_1.nsp")
   if destPath != expected {
      t.Errorf("expected dest %q, got %q", expected, destPath)
   }
}

func TestMoveToArchive_MultipleConflicts(t *testing.T) {
   dir := t.TempDir()
   archivePath := filepath.Join(dir, "archive")

   am, err := NewArchiveManager(archivePath, testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   if err := am.EnsureArchiveDir(); err != nil {
      t.Fatalf("EnsureArchiveDir failed: %v", err)
   }

   // Create game.nsp and game_1.nsp in archive.
   os.WriteFile(filepath.Join(archivePath, "game.nsp"), []byte("v0"), 0644)
   os.WriteFile(filepath.Join(archivePath, "game_1.nsp"), []byte("v1"), 0644)

   srcFile := filepath.Join(dir, "game.nsp")
   os.WriteFile(srcFile, []byte("v2"), 0644)

   destPath, err := am.MoveToArchive(srcFile)
   if err != nil {
      t.Fatalf("MoveToArchive failed: %v", err)
   }

   expected := filepath.Join(archivePath, "game_2.nsp")
   if destPath != expected {
      t.Errorf("expected dest %q, got %q", expected, destPath)
   }
}

func TestMoveToArchive_EmptySourcePath(t *testing.T) {
   dir := t.TempDir()
   am, err := NewArchiveManager(filepath.Join(dir, "archive"), testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   _, err = am.MoveToArchive("")
   if err == nil {
      t.Fatal("expected error for empty source path")
   }
   if !strings.Contains(err.Error(), "source path must not be empty") {
      t.Errorf("unexpected error: %v", err)
   }
}

func TestMoveToArchive_SourceNotExist(t *testing.T) {
   dir := t.TempDir()
   archivePath := filepath.Join(dir, "archive")

   am, err := NewArchiveManager(archivePath, testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   _, err = am.MoveToArchive(filepath.Join(dir, "nonexistent.nsp"))
   if err == nil {
      t.Fatal("expected error for non-existent source file")
   }
}

func TestMoveToArchive_ThreeConflicts(t *testing.T) {
   dir := t.TempDir()
   archivePath := filepath.Join(dir, "archive")

   am, err := NewArchiveManager(archivePath, testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   if err := am.EnsureArchiveDir(); err != nil {
      t.Fatalf("EnsureArchiveDir failed: %v", err)
   }

   // Create game.nsp, game_1.nsp, and game_2.nsp in archive.
   os.WriteFile(filepath.Join(archivePath, "game.nsp"), []byte("v0"), 0644)
   os.WriteFile(filepath.Join(archivePath, "game_1.nsp"), []byte("v1"), 0644)
   os.WriteFile(filepath.Join(archivePath, "game_2.nsp"), []byte("v2"), 0644)

   srcFile := filepath.Join(dir, "game.nsp")
   os.WriteFile(srcFile, []byte("v3"), 0644)

   destPath, err := am.MoveToArchive(srcFile)
   if err != nil {
      t.Fatalf("MoveToArchive failed: %v", err)
   }

   expected := filepath.Join(archivePath, "game_3.nsp")
   if destPath != expected {
      t.Errorf("expected dest %q, got %q", expected, destPath)
   }

   // Verify content preserved.
   data, err := os.ReadFile(destPath)
   if err != nil {
      t.Fatalf("failed to read destination: %v", err)
   }
   if string(data) != "v3" {
      t.Errorf("expected content %q, got %q", "v3", string(data))
   }
}

func TestEnsureArchiveDir_DeeplyNested(t *testing.T) {
   dir := t.TempDir()
   archivePath := filepath.Join(dir, "level1", "level2", "level3", "level4", "archive")

   am, err := NewArchiveManager(archivePath, testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   if err := am.EnsureArchiveDir(); err != nil {
      t.Fatalf("EnsureArchiveDir failed: %v", err)
   }

   info, err := os.Stat(archivePath)
   if err != nil {
      t.Fatalf("directory does not exist: %v", err)
   }
   if !info.IsDir() {
      t.Error("expected a directory")
   }
}

func TestEnsureArchiveDir_Idempotent(t *testing.T) {
   dir := t.TempDir()
   archivePath := filepath.Join(dir, "archive")

   am, err := NewArchiveManager(archivePath, testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   // Call twice — second call should succeed without error.
   if err := am.EnsureArchiveDir(); err != nil {
      t.Fatalf("first EnsureArchiveDir failed: %v", err)
   }
   if err := am.EnsureArchiveDir(); err != nil {
      t.Fatalf("second EnsureArchiveDir failed: %v", err)
   }
}

func TestResolveConflict_NoExtension(t *testing.T) {
   dir := t.TempDir()
   archivePath := filepath.Join(dir, "archive")

   am, err := NewArchiveManager(archivePath, testLogger())
   if err != nil {
      t.Fatalf("unexpected error: %v", err)
   }

   if err := am.EnsureArchiveDir(); err != nil {
      t.Fatalf("EnsureArchiveDir failed: %v", err)
   }

   // Create a file without extension.
   os.WriteFile(filepath.Join(archivePath, "README"), []byte("existing"), 0644)

   resolved, err := am.resolveConflict("README")
   if err != nil {
      t.Fatalf("resolveConflict failed: %v", err)
   }
   if resolved != "README_1" {
      t.Errorf("expected %q, got %q", "README_1", resolved)
   }
}
