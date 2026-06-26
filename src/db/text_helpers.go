package db

// TruncateDescription truncates text to a maximum length, appending an ellipsis
// character ("…") if the text exceeds maxLen. If the text length is less than or
// equal to maxLen, the full string is returned unchanged.
//
// This is used to limit description text in the Game Detail View to a readable
// length while indicating that additional content exists.
func TruncateDescription(text string, maxLen int) string {
   if maxLen < 0 {
      maxLen = 0
   }
   runes := []rune(text)
   if len(runes) <= maxLen {
      return text
   }
   return string(runes[:maxLen]) + "…"
}
