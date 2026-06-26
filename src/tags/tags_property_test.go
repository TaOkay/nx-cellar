package tags

import (
   "path/filepath"
   "regexp"
   "strings"
   "testing"
   "unicode"

   "pgregory.net/rapid"
)

// Feature: nx-cellar-enhancements, Property 6: Custom tag name validation
// **Validates: Requirements 4.1**
//
// Generate random strings (mix of valid and invalid characters, various lengths)
// and verify that ValidateTagName accepts if and only if the string matches
// ^[a-zA-Z0-9 -]{1,30}$
func TestProperty6_CustomTagNameValidation(t *testing.T) {
   tm := NewTagManager(t.TempDir())
   validPattern := regexp.MustCompile(`^[a-zA-Z0-9 -]{1,30}$`)

   rapid.Check(t, func(t *rapid.T) {
      // Generate random strings with a mix of valid and invalid chars and lengths (0-40).
      // Use a rune generator that includes valid tag chars plus some invalid ones.
      runeGen := rapid.RuneFrom(
         []rune{' ', '-', '!', '@', '#', '_', '/', '\n', '\t', '~', '.', ':', 'é'},
         &unicode.RangeTable{
            R16: []unicode.Range16{
               {Lo: 'a', Hi: 'z', Stride: 1},
               {Lo: 'A', Hi: 'Z', Stride: 1},
               {Lo: '0', Hi: '9', Stride: 1},
            },
         },
      )
      name := rapid.StringOfN(runeGen, 0, 40, -1).Draw(t, "name")

      err := tm.ValidateTagName(name)
      shouldAccept := validPattern.MatchString(name)

      if shouldAccept && err != nil {
         t.Fatalf("ValidateTagName(%q) returned error %v, but regex says it should be accepted", name, err)
      }
      if !shouldAccept && err == nil {
         t.Fatalf("ValidateTagName(%q) returned nil, but regex says it should be rejected", name)
      }
   })
}

// Feature: nx-cellar-enhancements, Property 4: Location tag resolution
// **Validates: Requirements 3.1, 3.3, 3.4, 3.6, 3.8**
//
// Generate random file paths and sets of scan folders with optional display names.
// Verify deepest matching folder is selected and correct display name returned.
func TestProperty4_LocationTagResolution(t *testing.T) {
   rapid.Check(t, func(t *rapid.T) {
      // Generate between 1 and 5 scan folders
      numFolders := rapid.IntRange(1, 5).Draw(t, "numFolders")

      // Build scan folders as absolute paths
      scanFolders := make([]string, numFolders)
      for i := 0; i < numFolders; i++ {
         depth := rapid.IntRange(1, 4).Draw(t, "depth")
         parts := make([]string, depth)
         for j := 0; j < depth; j++ {
            parts[j] = rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{1,10}`).Draw(t, "part")
         }
         scanFolders[i] = filepath.Join(append([]string{"/"}, parts...)...)
      }

      // Pick one scan folder as the parent for our file
      chosenIdx := rapid.IntRange(0, numFolders-1).Draw(t, "chosenIdx")
      chosenFolder := scanFolders[chosenIdx]

      // Generate a filename (child under the chosen folder)
      fileName := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{1,10}`).Draw(t, "fileName") + ".nsp"
      filePath := filepath.Join(chosenFolder, fileName)

      // Optionally assign display names to some folders
      tm := NewTagManager("/tmp/tags_property_test")
      displayNames := make(map[string]string)
      for i := 0; i < numFolders; i++ {
         if rapid.Bool().Draw(t, "hasDisplayName") {
            dn := rapid.StringMatching(`[A-Z][a-z]{2,15}`).Draw(t, "displayName")
            displayNames[scanFolders[i]] = dn
            tm.store.LocationDisplayNames[scanFolders[i]] = dn
         }
      }

      // Call GetLocationTag
      result := tm.GetLocationTag(filePath, scanFolders)

      // Determine the expected deepest matching folder
      cleanFile := filepath.Clean(filePath)
      bestMatch := ""
      for _, folder := range scanFolders {
         cleanFolder := filepath.Clean(folder)
         folderWithSep := cleanFolder
         if !strings.HasSuffix(folderWithSep, string(filepath.Separator)) {
            folderWithSep += string(filepath.Separator)
         }
         isChild := strings.HasPrefix(cleanFile, folderWithSep) || cleanFile == cleanFolder
         if isChild && len(cleanFolder) > len(bestMatch) {
            bestMatch = cleanFolder
         }
      }

      // There must always be a result for valid inputs (file is child of a scan folder)
      if result == "" {
         t.Fatalf("GetLocationTag(%q, %v) returned empty string, expected non-empty result", filePath, scanFolders)
      }

      // If bestMatch found, verify correct display name or base name
      if bestMatch != "" {
         if dn, ok := displayNames[bestMatch]; ok {
            if result != dn {
               t.Fatalf("GetLocationTag(%q, %v) = %q, expected display name %q for folder %q",
                  filePath, scanFolders, result, dn, bestMatch)
            }
         } else {
            expectedBase := filepath.Base(bestMatch)
            if result != expectedBase {
               t.Fatalf("GetLocationTag(%q, %v) = %q, expected base name %q for folder %q",
                  filePath, scanFolders, result, expectedBase, bestMatch)
            }
         }
      } else {
         // No matching folder - should return "Unknown"
         if result != "Unknown" {
            t.Fatalf("GetLocationTag(%q, %v) = %q, expected 'Unknown' (no matching folder)",
               filePath, scanFolders, result)
         }
      }
   })
}
