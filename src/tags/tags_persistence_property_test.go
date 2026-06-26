package tags

import (
   "strings"
   "testing"

   "go.uber.org/zap"
   "pgregory.net/rapid"
)

func init() {
   // Initialize a no-op logger so TagManager doesn't panic during tests.
   logger, _ := zap.NewDevelopment()
   zap.ReplaceGlobals(logger)
}

// validTagNameGen generates random valid tag names matching [a-zA-Z0-9 -]{1,30}.
func validTagNameGen() *rapid.Generator[string] {
   return rapid.Custom(func(t *rapid.T) string {
      chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 -"
      length := rapid.IntRange(1, 30).Draw(t, "tagNameLength")
      result := make([]byte, length)
      for i := range result {
         result[i] = chars[rapid.IntRange(0, len(chars)-1).Draw(t, "charIdx")]
      }
      return string(result)
   })
}

// titleIdPrefixGen generates random valid title ID prefixes (hex-like strings).
func titleIdPrefixGen() *rapid.Generator[string] {
   return rapid.Custom(func(t *rapid.T) string {
      chars := "0123456789ABCDEF"
      length := rapid.IntRange(8, 16).Draw(t, "titleIdLength")
      result := make([]byte, length)
      for i := range result {
         result[i] = chars[rapid.IntRange(0, len(chars)-1).Draw(t, "hexCharIdx")]
      }
      return string(result)
   })
}

// Feature: nx-cellar-enhancements, Property 7: Tag persistence round-trip
// **Validates: Requirements 4.2**
func TestProperty7_TagPersistenceRoundTrip(t *testing.T) {
   rapid.Check(t, func(rt *rapid.T) {
      tagName := validTagNameGen().Draw(rt, "tagName")
      titleId := titleIdPrefixGen().Draw(rt, "titleId")

      tmpDir := t.TempDir()

      // Create a TagManager, create a tag, and add it to a game
      tm1 := NewTagManager(tmpDir)
      err := tm1.Load()
      if err != nil {
         rt.Fatalf("Load failed on first manager: %v", err)
      }

      err = tm1.CreateTag(tagName)
      if err != nil {
         rt.Fatalf("CreateTag failed: %v", err)
      }

      err = tm1.AddTagToGame(titleId, tagName)
      if err != nil {
         rt.Fatalf("AddTagToGame failed: %v", err)
      }

      // Create a new TagManager pointing to the same folder and Load
      tm2 := NewTagManager(tmpDir)
      err = tm2.Load()
      if err != nil {
         rt.Fatalf("Load failed on second manager: %v", err)
      }

      // Verify the tag is present in AllTags
      allTags := tm2.GetAllTags()
      found := false
      for _, tag := range allTags {
         if tag == tagName {
            found = true
            break
         }
      }
      if !found {
         rt.Fatalf("tag %q not found in AllTags after reload; got %v", tagName, allTags)
      }

      // Verify the tag is associated with the game
      gameTags := tm2.GetGameTags(titleId)
      foundInGame := false
      for _, tag := range gameTags {
         if tag == tagName {
            foundInGame = true
            break
         }
      }
      if !foundInGame {
         rt.Fatalf("tag %q not found in game %q tags after reload; got %v", tagName, titleId, gameTags)
      }
   })
}

// Feature: nx-cellar-enhancements, Property 8: Tag uniqueness enforcement (case-insensitive)
// **Validates: Requirements 4.9**
func TestProperty8_TagUniquenessEnforcement(t *testing.T) {
   rapid.Check(t, func(rt *rapid.T) {
      tagName := validTagNameGen().Draw(rt, "tagName")

      tmpDir := t.TempDir()

      tm := NewTagManager(tmpDir)
      err := tm.Load()
      if err != nil {
         rt.Fatalf("Load failed: %v", err)
      }

      // Create the original tag
      err = tm.CreateTag(tagName)
      if err != nil {
         rt.Fatalf("CreateTag failed for original: %v", err)
      }

      // Attempt to create uppercase variant
      upper := strings.ToUpper(tagName)
      err = tm.CreateTag(upper)
      if err == nil {
         rt.Fatalf("expected error creating uppercase variant %q of existing tag %q, but got nil", upper, tagName)
      }

      // Attempt to create lowercase variant
      lower := strings.ToLower(tagName)
      err = tm.CreateTag(lower)
      if err == nil {
         rt.Fatalf("expected error creating lowercase variant %q of existing tag %q, but got nil", lower, tagName)
      }

      // Attempt to create mixed-case variant (swap case of each character)
      mixed := swapCase(tagName)
      err = tm.CreateTag(mixed)
      if err == nil {
         rt.Fatalf("expected error creating mixed-case variant %q of existing tag %q, but got nil", mixed, tagName)
      }

      // Verify only one tag exists
      allTags := tm.GetAllTags()
      if len(allTags) != 1 {
         rt.Fatalf("expected exactly 1 tag after duplicate rejections, got %d: %v", len(allTags), allTags)
      }
   })
}

// Feature: nx-cellar-enhancements, Property 9: Maximum tags per game invariant
// **Validates: Requirements 4.7**
func TestProperty9_MaxTagsPerGameInvariant(t *testing.T) {
   rapid.Check(t, func(rt *rapid.T) {
      titleId := titleIdPrefixGen().Draw(rt, "titleId")

      tmpDir := t.TempDir()

      tm := NewTagManager(tmpDir)
      err := tm.Load()
      if err != nil {
         rt.Fatalf("Load failed: %v", err)
      }

      // Use deterministic unique tag names to ensure 21 distinct tags
      tagNames := make([]string, 21)
      for i := 0; i < 21; i++ {
         tagNames[i] = "tag" + string(rune('A'+i/10)) + string(rune('a'+i%10))
      }

      // Create all 21 tags
      for i, name := range tagNames {
         err = tm.CreateTag(name)
         if err != nil {
            rt.Fatalf("CreateTag failed for tag %d (%q): %v", i, name, err)
         }
      }

      // Add first 20 tags to the game
      for i := 0; i < 20; i++ {
         err = tm.AddTagToGame(titleId, tagNames[i])
         if err != nil {
            rt.Fatalf("AddTagToGame failed for tag %d (%q): %v", i, tagNames[i], err)
         }
      }

      // Verify game has exactly 20 tags
      gameTags := tm.GetGameTags(titleId)
      if len(gameTags) != 20 {
         rt.Fatalf("expected 20 tags on game, got %d", len(gameTags))
      }

      // Attempt to add the 21st tag - should be rejected
      err = tm.AddTagToGame(titleId, tagNames[20])
      if err == nil {
         rt.Fatalf("expected error adding 21st tag %q to game %q, but got nil", tagNames[20], titleId)
      }

      // Verify game still has exactly 20 tags
      gameTags = tm.GetGameTags(titleId)
      if len(gameTags) != 20 {
         rt.Fatalf("expected game to still have 20 tags after rejected 21st add, got %d", len(gameTags))
      }
   })
}

// Feature: nx-cellar-enhancements, Property 10: Cascade tag deletion
// **Validates: Requirements 4.8**
func TestProperty10_CascadeTagDeletion(t *testing.T) {
   rapid.Check(t, func(rt *rapid.T) {
      tagName := validTagNameGen().Draw(rt, "tagName")
      numGames := rapid.IntRange(1, 10).Draw(rt, "numGames")

      tmpDir := t.TempDir()

      tm := NewTagManager(tmpDir)
      err := tm.Load()
      if err != nil {
         rt.Fatalf("Load failed: %v", err)
      }

      // Create the tag
      err = tm.CreateTag(tagName)
      if err != nil {
         rt.Fatalf("CreateTag failed: %v", err)
      }

      // Assign the tag to multiple random games
      gameIds := make([]string, numGames)
      for i := 0; i < numGames; i++ {
         gameIds[i] = titleIdPrefixGen().Draw(rt, "gameId")
         err = tm.AddTagToGame(gameIds[i], tagName)
         if err != nil {
            rt.Fatalf("AddTagToGame failed for game %q: %v", gameIds[i], err)
         }
      }

      // Verify tag is assigned to all games before deletion
      for _, gid := range gameIds {
         tags := tm.GetGameTags(gid)
         found := false
         for _, tg := range tags {
            if strings.EqualFold(tg, tagName) {
               found = true
               break
            }
         }
         if !found {
            rt.Fatalf("tag %q not found on game %q before deletion", tagName, gid)
         }
      }

      // Delete the tag
      err = tm.DeleteTag(tagName)
      if err != nil {
         rt.Fatalf("DeleteTag failed: %v", err)
      }

      // Verify tag is removed from AllTags
      allTags := tm.GetAllTags()
      for _, tag := range allTags {
         if strings.EqualFold(tag, tagName) {
            rt.Fatalf("tag %q still present in AllTags after deletion", tagName)
         }
      }

      // Verify no game has the tag anymore
      for _, gid := range gameIds {
         gameTags := tm.GetGameTags(gid)
         for _, tg := range gameTags {
            if strings.EqualFold(tg, tagName) {
               rt.Fatalf("tag %q still present on game %q after cascade deletion", tagName, gid)
            }
         }
      }
   })
}

// swapCase inverts the case of each letter in a string.
func swapCase(s string) string {
   result := make([]byte, len(s))
   for i := 0; i < len(s); i++ {
      c := s[i]
      if c >= 'a' && c <= 'z' {
         result[i] = c - 32
      } else if c >= 'A' && c <= 'Z' {
         result[i] = c + 32
      } else {
         result[i] = c
      }
   }
   return string(result)
}
