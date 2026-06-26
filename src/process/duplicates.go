package process

import (
   "fmt"
   "path/filepath"
   "strings"

   "github.com/trembon/switch-library-manager/db"
)

// DuplicateFile represents a single file that is part of a duplicate group.
type DuplicateFile struct {
   FileName     string `json:"file_name"`
   Path         string `json:"path"`
   Size         int64  `json:"size"`
   FileType     string `json:"file_type"`     // "nsp", "nsz", "xci", "xcz"
   IsCompressed bool   `json:"is_compressed"` // true for nsz, xcz
   Recommended  bool   `json:"recommended"`
}

// DuplicateGroup represents a set of duplicate files for the same title.
type DuplicateGroup struct {
   TitleId   string          `json:"title_id"`
   TitleName string          `json:"title_name"`
   Files     []DuplicateFile `json:"files"`
}

// DuplicateResult contains all detected duplicate groups and summary statistics.
type DuplicateResult struct {
   Groups          []DuplicateGroup `json:"groups"`
   TotalGroups     int              `json:"total_groups"`
   ReclaimableSize int64            `json:"reclaimable_size"`
}

// DetectDuplicates scans the local database for base game files that have
// duplicates tracked in the skipped map with REASON_DUPLICATE.
// It returns a DuplicateResult with groups of duplicate files and a recommendation
// for which file to keep in each group.
func DetectDuplicates(localDB *db.LocalSwitchFilesDB, titlesDB *db.SwitchTitlesDB) *DuplicateResult {
   result := &DuplicateResult{
      Groups: []DuplicateGroup{},
   }

   // Build a map of title ID prefix -> skipped duplicate files
   // We need to associate skipped files with their title ID prefix
   duplicatesByPrefix := map[string][]db.ExtendedFileInfo{}

   for fileInfo, skipped := range localDB.Skipped {
      if skipped.ReasonCode != db.REASON_DUPLICATE {
         continue
      }

      // Determine the title ID prefix from the file name
      titleId := parseTitleIdFromFile(fileInfo.FileName)
      if titleId == "" {
         continue
      }

      idPrefix := titleIdToPrefix(titleId)
      duplicatesByPrefix[idPrefix] = append(duplicatesByPrefix[idPrefix], fileInfo)
   }

   // For each prefix that has duplicates, build a DuplicateGroup
   for idPrefix, skippedFiles := range duplicatesByPrefix {
      gameFiles, exists := localDB.TitlesMap[idPrefix]
      if !exists || !gameFiles.BaseExist {
         continue
      }

      var files []DuplicateFile

      // Add the base file currently tracked in TitlesMap
      baseFile := gameFiles.File.ExtendedInfo
      files = append(files, toDuplicateFile(baseFile))

      // Add the skipped duplicate files
      for _, skippedFile := range skippedFiles {
         files = append(files, toDuplicateFile(skippedFile))
      }

      // Only include groups with more than 1 file
      if len(files) < 2 {
         continue
      }

      // Apply recommendation
      recommendedIdx := RecommendKeep(files)
      files[recommendedIdx].Recommended = true

      // Resolve the title name
      titleName := resolveTitleName(idPrefix, gameFiles, titlesDB)

      group := DuplicateGroup{
         TitleId:   idPrefix,
         TitleName: titleName,
         Files:     files,
      }
      result.Groups = append(result.Groups, group)
   }

   result.TotalGroups = len(result.Groups)
   result.ReclaimableSize = calculateReclaimableSize(result.Groups)

   return result
}

// RecommendKeep determines which file in a duplicate group should be kept.
// It returns the index of the recommended file based on:
// 1. Prefer compressed formats (.nsz, .xcz) over uncompressed (.nsp, .xci)
// 2. Among same compression status, prefer larger file size
func RecommendKeep(files []DuplicateFile) int {
   if len(files) == 0 {
      return 0
   }

   bestIdx := 0
   for i := 1; i < len(files); i++ {
      if isBetterFile(files[i], files[bestIdx]) {
         bestIdx = i
      }
   }
   return bestIdx
}

// isBetterFile returns true if candidate is preferred over current.
func isBetterFile(candidate, current DuplicateFile) bool {
   // Prefer compressed over uncompressed
   if candidate.IsCompressed && !current.IsCompressed {
      return true
   }
   if !candidate.IsCompressed && current.IsCompressed {
      return false
   }
   // Among same compression status, prefer larger file size
   return candidate.Size > current.Size
}

// calculateReclaimableSize sums the sizes of all non-recommended files across groups.
func calculateReclaimableSize(groups []DuplicateGroup) int64 {
   var total int64
   for _, group := range groups {
      for _, file := range group.Files {
         if !file.Recommended {
            total += file.Size
         }
      }
   }
   return total
}

// toDuplicateFile converts an ExtendedFileInfo to a DuplicateFile.
func toDuplicateFile(info db.ExtendedFileInfo) DuplicateFile {
   ext := strings.ToLower(filepath.Ext(info.FileName))
   // Strip the leading dot for file_type
   fileType := strings.TrimPrefix(ext, ".")

   isCompressed := ext == ".nsz" || ext == ".xcz"

   return DuplicateFile{
      FileName:     info.FileName,
      Path:         filepath.Join(info.BaseFolder, info.FileName),
      Size:         info.Size,
      FileType:     fileType,
      IsCompressed: isCompressed,
      Recommended:  false,
   }
}

// resolveTitleName attempts to find a human-readable name for the title.
// It checks the titles database first, then falls back to parsing the filename.
func resolveTitleName(idPrefix string, gameFiles *db.SwitchGameFiles, titlesDB *db.SwitchTitlesDB) string {
   if titlesDB != nil {
      if title, ok := titlesDB.TitlesMap[idPrefix]; ok && title.Attributes.Name != "" {
         return title.Attributes.Name
      }
   }
   // Fallback: parse the name from the base file's filename
   return db.ParseTitleNameFromFileName(gameFiles.File.ExtendedInfo.FileName)
}

// parseTitleIdFromFile extracts the title ID from a filename using bracket notation [titleId].
func parseTitleIdFromFile(fileName string) string {
   // Look for a 16-character hex string in brackets
   start := -1
   for i := 0; i < len(fileName); i++ {
      if fileName[i] == '[' {
         start = i + 1
      } else if fileName[i] == ']' && start != -1 {
         candidate := fileName[start:i]
         if len(candidate) == 16 && isHex(candidate) {
            return strings.ToLower(candidate)
         }
         start = -1
      }
   }
   return ""
}

// titleIdToPrefix converts a full title ID to the prefix used as the map key.
// The prefix logic matches localSwitchFilesDB.go: base/update use first 13 chars,
// DLC adjusts the 4th-from-right character.
func titleIdToPrefix(titleId string) string {
   if len(titleId) < 16 {
      return titleId
   }

   if strings.HasSuffix(titleId, "000") || strings.HasSuffix(titleId, "800") {
      return titleId[0 : len(titleId)-3]
   }

   // DLC: subtract 1 from the 4th-from-right hex digit
   pos := len(titleId) - 4
   ch := titleId[pos]
   var val uint64
   if ch >= '0' && ch <= '9' {
      val = uint64(ch - '0')
   } else if ch >= 'a' && ch <= 'f' {
      val = uint64(ch-'a') + 10
   }
   adjusted := strings.ToLower(fmt.Sprintf("%x", val-1))
   return titleId[0:pos] + adjusted
}

// isHex checks if a string contains only hexadecimal characters.
func isHex(s string) bool {
   for _, c := range s {
      if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
         return false
      }
   }
   return true
}
