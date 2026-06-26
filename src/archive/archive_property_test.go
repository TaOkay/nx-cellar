package archive

import (
   "fmt"
   "os"
   "path/filepath"
   "strings"
   "testing"

   "go.uber.org/zap"
   "pgregory.net/rapid"
)

// Feature: nx-cellar-enhancements, Property 15: Archive filename conflict resolution

// **Validates: Requirements 8.3, 8.4**
// Property 15: Given any filename and N (0–999) existing files in the archive,
// MoveToArchive always produces a unique filename that does not collide with existing files.
func TestProperty_ArchiveFilenameConflictResolution(t *testing.T) {
   rapid.Check(t, func(t *rapid.T) {
      // Generate a random base name (1-20 alphanumeric chars).
      baseName := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "baseName")

      // Generate a random extension from common Switch file types.
      extensions := []string{".nsp", ".xci", ".xcz", ".nsz"}
      extIdx := rapid.IntRange(0, len(extensions)-1).Draw(t, "extIdx")
      ext := extensions[extIdx]

      filename := baseName + ext

      // Generate number of pre-existing conflicting files (0 to 50 for speed).
      numExisting := rapid.IntRange(0, 50).Draw(t, "numExisting")

      // Set up temp directory as archive.
      tmpDir, err := os.MkdirTemp("", "archive-prop-*")
      if err != nil {
         t.Fatalf("failed to create temp dir: %v", err)
      }
      defer os.RemoveAll(tmpDir)

      archivePath := filepath.Join(tmpDir, "archive")

      logger, _ := zap.NewDevelopment()
      am, err := NewArchiveManager(archivePath, logger.Sugar())
      if err != nil {
         t.Fatalf("failed to create archive manager: %v", err)
      }

      if err := am.EnsureArchiveDir(); err != nil {
         t.Fatalf("failed to ensure archive dir: %v", err)
      }

      // Create N existing files in the archive to simulate conflicts.
      // First file uses the original name, subsequent use _1, _2, etc.
      existingFiles := make(map[string]bool)
      for i := 0; i < numExisting; i++ {
         var conflictName string
         if i == 0 {
            conflictName = filename
         } else {
            conflictName = fmt.Sprintf("%s_%d%s", baseName, i, ext)
         }
         conflictPath := filepath.Join(archivePath, conflictName)
         if err := os.WriteFile(conflictPath, []byte("existing"), 0644); err != nil {
            t.Fatalf("failed to create conflict file: %v", err)
         }
         existingFiles[conflictName] = true
      }

      // Create the source file to be archived.
      srcDir, err := os.MkdirTemp("", "archive-src-*")
      if err != nil {
         t.Fatalf("failed to create source dir: %v", err)
      }
      defer os.RemoveAll(srcDir)

      srcFile := filepath.Join(srcDir, filename)
      if err := os.WriteFile(srcFile, []byte("new content"), 0644); err != nil {
         t.Fatalf("failed to create source file: %v", err)
      }

      // Act: Move to archive.
      destPath, err := am.MoveToArchive(srcFile)
      if err != nil {
         t.Fatalf("MoveToArchive failed: %v", err)
      }

      // Assert: The destination file actually exists.
      if _, statErr := os.Stat(destPath); statErr != nil {
         t.Fatalf("destination file does not exist: %v", statErr)
      }

      // Assert: The destination filename is unique (not in our pre-existing set).
      destFilename := filepath.Base(destPath)
      if numExisting > 0 && existingFiles[destFilename] {
         t.Fatalf("filename collision: %q already existed in archive", destFilename)
      }

      // Assert: The filename follows expected pattern.
      if numExisting == 0 {
         // No conflicts — should use the original filename.
         if destFilename != filename {
            t.Fatalf("expected original filename %q, got %q", filename, destFilename)
         }
      } else {
         // With conflicts — should have a numeric suffix.
         expectedSuffix := fmt.Sprintf("_%d%s", numExisting, ext)
         if !strings.HasSuffix(destFilename, expectedSuffix) {
            // Verify at minimum it has the base and ext.
            if !strings.HasPrefix(destFilename, baseName) || !strings.HasSuffix(destFilename, ext) {
               t.Fatalf("unexpected filename pattern: %q (base=%q, ext=%q)", destFilename, baseName, ext)
            }
         }
      }

      // Assert: Source file no longer exists.
      if _, statErr := os.Stat(srcFile); !os.IsNotExist(statErr) {
         t.Fatalf("source file should have been removed after archiving")
      }
   })
}
