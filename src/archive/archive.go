package archive

import (
   "errors"
   "fmt"
   "io"
   "os"
   "path/filepath"
   "strings"

   "go.uber.org/zap"
)

const (
   // maxPathLength is the maximum allowed length for the archive path.
   maxPathLength = 260
   // maxConflictAttempts is the maximum number of suffix attempts for conflict resolution.
   maxConflictAttempts = 999
)

// ArchiveManager handles moving files to an archive directory with conflict resolution.
type ArchiveManager struct {
   archivePath string
   logger      *zap.SugaredLogger
}

// NewArchiveManager creates a new ArchiveManager with path validation.
// The archivePath must be non-empty, absolute, and at most 260 characters.
func NewArchiveManager(archivePath string, logger *zap.SugaredLogger) (*ArchiveManager, error) {
   if archivePath == "" {
      return nil, errors.New("archive path must not be empty")
   }
   if !filepath.IsAbs(archivePath) {
      return nil, fmt.Errorf("archive path must be absolute, got %q", archivePath)
   }
   if len(archivePath) > maxPathLength {
      return nil, fmt.Errorf("archive path must be at most %d characters, got %d", maxPathLength, len(archivePath))
   }
   if logger == nil {
      return nil, errors.New("logger must not be nil")
   }

   return &ArchiveManager{
      archivePath: archivePath,
      logger:      logger,
   }, nil
}

// EnsureArchiveDir creates the archive directory (including parents) if it does not exist.
func (am *ArchiveManager) EnsureArchiveDir() error {
   if err := os.MkdirAll(am.archivePath, 0755); err != nil {
      return fmt.Errorf("failed to create archive directory %q: %w", am.archivePath, err)
   }
   return nil
}

// MoveToArchive moves a file from sourcePath into the archive directory.
// If a file with the same name already exists in the archive, conflict resolution
// appends a numeric suffix (_1 through _999) before the extension.
// Returns the final destination path on success.
func (am *ArchiveManager) MoveToArchive(sourcePath string) (string, error) {
   if sourcePath == "" {
      return "", errors.New("source path must not be empty")
   }

   // Ensure the archive directory exists before moving.
   if err := am.EnsureArchiveDir(); err != nil {
      return "", err
   }

   filename := filepath.Base(sourcePath)
   destPath := filepath.Join(am.archivePath, filename)

   // Check if destination already exists; if so, resolve the conflict.
   if _, err := os.Stat(destPath); err == nil {
      resolved, resolveErr := am.resolveConflict(filename)
      if resolveErr != nil {
         return "", resolveErr
      }
      destPath = filepath.Join(am.archivePath, resolved)
   }

   am.logger.Infof("Moving file to archive: %q -> %q", sourcePath, destPath)

   // Attempt os.Rename first (fast, same-device move).
   if err := os.Rename(sourcePath, destPath); err != nil {
      // Cross-device fallback: copy then delete.
      if copyErr := copyFile(sourcePath, destPath); copyErr != nil {
         return "", fmt.Errorf("failed to move %q to %q: %w", sourcePath, destPath, copyErr)
      }
      if removeErr := os.Remove(sourcePath); removeErr != nil {
         // File was copied successfully but source removal failed — log but don't fail.
         am.logger.Warnf("File copied to archive but failed to remove source %q: %v", sourcePath, removeErr)
      }
   }

   am.logger.Infof("Successfully archived: %q -> %q", sourcePath, destPath)
   return destPath, nil
}

// resolveConflict finds an available filename in the archive directory by appending
// a numeric suffix (_1 through _999) before the file extension.
// Given "game.nsp" it tries "game_1.nsp", "game_2.nsp", etc.
func (am *ArchiveManager) resolveConflict(filename string) (string, error) {
   ext := filepath.Ext(filename)
   base := strings.TrimSuffix(filename, ext)

   for i := 1; i <= maxConflictAttempts; i++ {
      candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
      candidatePath := filepath.Join(am.archivePath, candidate)
      if _, err := os.Stat(candidatePath); errors.Is(err, os.ErrNotExist) {
         return candidate, nil
      }
   }

   return "", fmt.Errorf("max conflict resolution attempts (%d) exceeded for %q", maxConflictAttempts, filename)
}

// copyFile copies a file from src to dst. It creates the destination file with
// the same permissions as the source.
func copyFile(src, dst string) error {
   srcFile, err := os.Open(src)
   if err != nil {
      return fmt.Errorf("failed to open source file %q: %w", src, err)
   }
   defer srcFile.Close()

   srcInfo, err := srcFile.Stat()
   if err != nil {
      return fmt.Errorf("failed to stat source file %q: %w", src, err)
   }

   dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
   if err != nil {
      return fmt.Errorf("failed to create destination file %q: %w", dst, err)
   }
   defer dstFile.Close()

   if _, err := io.Copy(dstFile, srcFile); err != nil {
      // Clean up partial file on copy failure.
      os.Remove(dst)
      return fmt.Errorf("failed to copy data from %q to %q: %w", src, dst, err)
   }

   return nil
}
