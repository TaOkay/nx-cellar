package process

import (
   "fmt"
   "strings"
   "testing"

   "github.com/trembon/switch-library-manager/db"
   "pgregory.net/rapid"
)

// --- Generators ---

// hexChar generates a single lowercase hex character.
func hexCharGen() *rapid.Generator[byte] {
   return rapid.Map(rapid.IntRange(0, 15), func(i int) byte {
      if i < 10 {
         return byte('0' + i)
      }
      return byte('a' + i - 10)
   })
}

// titleIdPrefixGen generates a 13-character lowercase hex string (title ID prefix).
func titleIdPrefixGen() *rapid.Generator[string] {
   return rapid.Custom[string](func(t *rapid.T) string {
      var sb strings.Builder
      for i := 0; i < 13; i++ {
         sb.WriteByte(hexCharGen().Draw(t, fmt.Sprintf("hex_%d", i)))
      }
      return sb.String()
   })
}

// fileExtGen generates one of the 4 supported extensions.
func fileExtGen() *rapid.Generator[string] {
   return rapid.SampledFrom([]string{".nsp", ".nsz", ".xci", ".xcz"})
}

// duplicateFileGen generates a random DuplicateFile.
func duplicateFileGen() *rapid.Generator[DuplicateFile] {
   return rapid.Custom[DuplicateFile](func(t *rapid.T) DuplicateFile {
      ext := fileExtGen().Draw(t, "ext")
      isCompressed := ext == ".nsz" || ext == ".xcz"
      baseName := rapid.StringMatching(`[A-Za-z][A-Za-z0-9]{2,15}`).Draw(t, "baseName")
      size := rapid.Int64Range(1, 10*1024*1024*1024).Draw(t, "size")
      path := "/games/" + baseName + ext

      return DuplicateFile{
         FileName:     baseName + ext,
         Path:         path,
         Size:         size,
         FileType:     strings.TrimPrefix(ext, "."),
         IsCompressed: isCompressed,
         Recommended:  false,
      }
   })
}

// --- Property 16: Duplicate detection correctness ---

// Feature: nx-cellar-enhancements, Property 16: Duplicate detection correctness
// **Validates: Requirements 9.2**
//
// Generate random library states with various duplicate scenarios.
// Verify exact set of duplicate title IDs detected (no false positives/negatives).
func TestProperty16_DuplicateDetectionCorrectness(t *testing.T) {
   rapid.Check(t, func(rt *rapid.T) {
      // Generate a number of title prefixes (some will have duplicates, some won't)
      numTitles := rapid.IntRange(1, 10).Draw(rt, "numTitles")
      prefixes := make([]string, numTitles)
      for i := 0; i < numTitles; i++ {
         prefixes[i] = titleIdPrefixGen().Draw(rt, fmt.Sprintf("prefix_%d", i))
      }

      titlesMap := map[string]*db.SwitchGameFiles{}
      skipped := map[db.ExtendedFileInfo]db.SkippedFile{}

      // Track which prefixes should produce duplicate groups
      expectedDuplicatePrefixes := map[string]bool{}

      for i, prefix := range prefixes {
         hasBase := rapid.Bool().Draw(rt, fmt.Sprintf("hasBase_%d", i))
         numDuplicates := rapid.IntRange(0, 3).Draw(rt, fmt.Sprintf("numDups_%d", i))
         numOtherSkipped := rapid.IntRange(0, 2).Draw(rt, fmt.Sprintf("numOther_%d", i))

         // Create TitlesMap entry
         ext := fileExtGen().Draw(rt, fmt.Sprintf("baseExt_%d", i))
         baseFileName := fmt.Sprintf("Game%d [%s000]%s", i, prefix, ext)
         baseFile := db.ExtendedFileInfo{
            FileName:   baseFileName,
            BaseFolder: "/games/",
            Size:       rapid.Int64Range(100, 5000000000).Draw(rt, fmt.Sprintf("baseSize_%d", i)),
         }

         gameFiles := &db.SwitchGameFiles{
            File:      db.SwitchFileInfo{ExtendedInfo: baseFile},
            BaseExist: hasBase,
            Updates:   map[int]db.SwitchFileInfo{},
            Dlc:       map[string]db.SwitchFileInfo{},
         }
         titlesMap[prefix] = gameFiles

         // Add duplicate skipped files (with REASON_DUPLICATE and matching title prefix)
         for j := 0; j < numDuplicates; j++ {
            dupExt := fileExtGen().Draw(rt, fmt.Sprintf("dupExt_%d_%d", i, j))
            dupFileName := fmt.Sprintf("GameDup%d_%d [%s000]%s", i, j, prefix, dupExt)
            dupFileInfo := db.ExtendedFileInfo{
               FileName:   dupFileName,
               BaseFolder: fmt.Sprintf("/games/folder%d/", j),
               Size:       rapid.Int64Range(100, 5000000000).Draw(rt, fmt.Sprintf("dupSize_%d_%d", i, j)),
            }
            skipped[dupFileInfo] = db.SkippedFile{
               ReasonCode: db.REASON_DUPLICATE,
               ReasonText: "Duplicate base file",
            }
         }

         // Add non-duplicate skipped files (various other reason codes)
         for j := 0; j < numOtherSkipped; j++ {
            otherReason := rapid.SampledFrom([]int{
               db.REASON_UNSUPPORTED_TYPE,
               db.REASON_OLD_UPDATE,
               db.REASON_UNRECOGNISED,
               db.REASON_MALFORMED_FILE,
            }).Draw(rt, fmt.Sprintf("otherReason_%d_%d", i, j))
            otherFileName := fmt.Sprintf("Other%d_%d [%s000].nsp", i, j, prefix)
            otherFileInfo := db.ExtendedFileInfo{
               FileName:   otherFileName,
               BaseFolder: fmt.Sprintf("/other/folder%d/", j),
               Size:       rapid.Int64Range(100, 1000000).Draw(rt, fmt.Sprintf("otherSize_%d_%d", i, j)),
            }
            skipped[otherFileInfo] = db.SkippedFile{
               ReasonCode: otherReason,
               ReasonText: "Some other reason",
            }
         }

         // A title should appear as a duplicate group if:
         // 1. BaseExist is true AND
         // 2. There is at least one REASON_DUPLICATE entry matching this prefix
         // 3. And the resulting group has at least 2 files (base + duplicates)
         if hasBase && numDuplicates > 0 {
            expectedDuplicatePrefixes[prefix] = true
         }
      }

      // Create the LocalSwitchFilesDB
      localDB := &db.LocalSwitchFilesDB{
         TitlesMap: titlesMap,
         Skipped:   skipped,
      }

      // Call DetectDuplicates
      result := DetectDuplicates(localDB, nil)

      // Verify: collect detected prefixes
      detectedPrefixes := map[string]bool{}
      for _, group := range result.Groups {
         detectedPrefixes[group.TitleId] = true
      }

      // No false negatives: every expected duplicate should be detected
      for prefix := range expectedDuplicatePrefixes {
         if !detectedPrefixes[prefix] {
            rt.Fatalf("False negative: expected duplicate group for prefix %q but not detected", prefix)
         }
      }

      // No false positives: every detected duplicate should be expected
      for prefix := range detectedPrefixes {
         if !expectedDuplicatePrefixes[prefix] {
            rt.Fatalf("False positive: detected duplicate group for prefix %q but not expected", prefix)
         }
      }

      // Verify total groups matches
      if result.TotalGroups != len(expectedDuplicatePrefixes) {
         rt.Fatalf("TotalGroups = %d, expected %d", result.TotalGroups, len(expectedDuplicatePrefixes))
      }
   })
}

// --- Property 17: Duplicate recommendation algorithm ---

// Feature: nx-cellar-enhancements, Property 17: Duplicate recommendation algorithm
// **Validates: Requirements 9.6**
//
// Generate random file sets with mixed compression/sizes.
// Verify recommended file is compressed if available, else largest.
func TestProperty17_DuplicateRecommendationAlgorithm(t *testing.T) {
   rapid.Check(t, func(rt *rapid.T) {
      // Generate 2-8 random DuplicateFiles
      numFiles := rapid.IntRange(2, 8).Draw(rt, "numFiles")
      files := make([]DuplicateFile, numFiles)
      for i := 0; i < numFiles; i++ {
         files[i] = duplicateFileGen().Draw(rt, fmt.Sprintf("file_%d", i))
      }

      // Call RecommendKeep
      recommendedIdx := RecommendKeep(files)

      // Verify the recommendation is within bounds
      if recommendedIdx < 0 || recommendedIdx >= len(files) {
         rt.Fatalf("RecommendKeep returned index %d out of bounds [0, %d)", recommendedIdx, len(files))
      }

      recommended := files[recommendedIdx]

      // Check: if any compressed file exists, recommended must be compressed
      hasCompressed := false
      for _, f := range files {
         if f.IsCompressed {
            hasCompressed = true
            break
         }
      }

      if hasCompressed && !recommended.IsCompressed {
         rt.Fatalf("Compressed files exist but recommended file %q is not compressed", recommended.FileName)
      }

      // Check: among files with same compression status as recommended,
      // recommended has the largest size
      for i, f := range files {
         if i == recommendedIdx {
            continue
         }
         if f.IsCompressed == recommended.IsCompressed && f.Size > recommended.Size {
            rt.Fatalf("File %q (size=%d, compressed=%v) is larger than recommended %q (size=%d, compressed=%v) with same compression status",
               f.FileName, f.Size, f.IsCompressed,
               recommended.FileName, recommended.Size, recommended.IsCompressed)
         }
      }
   })
}

// --- Property 18: Reclaimable space calculation ---

// Feature: nx-cellar-enhancements, Property 18: Reclaimable space calculation
// **Validates: Requirements 9.8**
//
// Generate groups with selections, verify sum of non-selected file sizes.
func TestProperty18_ReclaimableSpaceCalculation(t *testing.T) {
   rapid.Check(t, func(rt *rapid.T) {
      // Generate 1-5 duplicate groups
      numGroups := rapid.IntRange(1, 5).Draw(rt, "numGroups")
      groups := make([]DuplicateGroup, numGroups)

      var expectedReclaimable int64

      for g := 0; g < numGroups; g++ {
         // Each group has 2-6 files
         numFiles := rapid.IntRange(2, 6).Draw(rt, fmt.Sprintf("numFiles_%d", g))
         files := make([]DuplicateFile, numFiles)
         for i := 0; i < numFiles; i++ {
            files[i] = duplicateFileGen().Draw(rt, fmt.Sprintf("file_%d_%d", g, i))
            files[i].Recommended = false
         }

         // Use RecommendKeep to select the recommended file
         recommendedIdx := RecommendKeep(files)
         files[recommendedIdx].Recommended = true

         // Calculate expected reclaimable: sum of non-recommended file sizes
         for i, f := range files {
            if i != recommendedIdx {
               expectedReclaimable += f.Size
            }
         }

         prefix := titleIdPrefixGen().Draw(rt, fmt.Sprintf("groupPrefix_%d", g))
         groups[g] = DuplicateGroup{
            TitleId:   prefix,
            TitleName: fmt.Sprintf("Game %d", g),
            Files:     files,
         }
      }

      // Call calculateReclaimableSize
      actualReclaimable := calculateReclaimableSize(groups)

      if actualReclaimable != expectedReclaimable {
         rt.Fatalf("calculateReclaimableSize = %d, expected %d", actualReclaimable, expectedReclaimable)
      }
   })
}
