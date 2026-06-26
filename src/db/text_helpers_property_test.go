package db

import (
   "testing"
   "unicode/utf8"

   "pgregory.net/rapid"
)

// Feature: nx-cellar-enhancements, Property 13: Description truncation
// **Validates: Requirements 7.4**
//
// Generate random-length strings and a random maxLen, verify correct truncation
// behavior at boundary: unchanged when within limit, truncated with "…" appended
// when exceeding it.
func TestProperty13_DescriptionTruncation(t *testing.T) {
   rapid.Check(t, func(t *rapid.T) {
      // Generate random strings of varying lengths (0 to 2000+ runes).
      // Use a broad rune generator including ASCII and multi-byte characters.
      textLen := rapid.IntRange(0, 2500).Draw(t, "textLen")
      runeGen := rapid.Rune()
      text := rapid.StringOfN(runeGen, textLen, textLen, -1).Draw(t, "text")

      // Use a random maxLen (0 to 1500).
      maxLen := rapid.IntRange(0, 1500).Draw(t, "maxLen")

      result := TruncateDescription(text, maxLen)

      textRunes := []rune(text)
      resultRunes := []rune(result)

      if len(textRunes) <= maxLen {
         // Text fits within maxLen: result must be the original text unchanged.
         if result != text {
            t.Fatalf("TruncateDescription(%q, %d): text fits (len %d <= %d) but result differs.\nGot:  %q\nWant: %q",
               text, maxLen, len(textRunes), maxLen, result, text)
         }
      } else {
         // Text exceeds maxLen: result must be first maxLen runes + "…".
         expectedPrefix := string(textRunes[:maxLen])
         expected := expectedPrefix + "…"

         if result != expected {
            t.Fatalf("TruncateDescription(text, %d): text exceeds limit (len %d > %d) but result is wrong.\nGot:  %q\nWant: %q",
               maxLen, len(textRunes), maxLen, result, expected)
         }

         // Verify result length in runes is maxLen + 1 (for the "…").
         if len(resultRunes) != maxLen+1 {
            t.Fatalf("TruncateDescription(text, %d): expected result rune length %d, got %d",
               maxLen, maxLen+1, len(resultRunes))
         }

         // Verify the last rune is the ellipsis character.
         if resultRunes[len(resultRunes)-1] != '…' {
            t.Fatalf("TruncateDescription(text, %d): expected last rune to be '…', got %q",
               maxLen, string(resultRunes[len(resultRunes)-1]))
         }
      }

      // Invariant: result must always be valid UTF-8.
      if !utf8.ValidString(result) {
         t.Fatalf("TruncateDescription(%q, %d): result is not valid UTF-8", text, maxLen)
      }
   })
}
