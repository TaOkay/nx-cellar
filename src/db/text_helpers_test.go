package db

import "testing"

func TestTruncateDescription(t *testing.T) {
   tests := []struct {
      name   string
      text   string
      maxLen int
      want   string
   }{
      {
         name:   "short text within limit",
         text:   "Hello",
         maxLen: 10,
         want:   "Hello",
      },
      {
         name:   "text exactly at limit",
         text:   "Hello",
         maxLen: 5,
         want:   "Hello",
      },
      {
         name:   "text exceeds limit",
         text:   "Hello, World!",
         maxLen: 5,
         want:   "Hello…",
      },
      {
         name:   "empty string",
         text:   "",
         maxLen: 10,
         want:   "",
      },
      {
         name:   "maxLen zero with non-empty text",
         text:   "Hello",
         maxLen: 0,
         want:   "…",
      },
      {
         name:   "maxLen zero with empty text",
         text:   "",
         maxLen: 0,
         want:   "",
      },
      {
         name:   "negative maxLen treated as zero",
         text:   "Hello",
         maxLen: -1,
         want:   "…",
      },
      {
         name:   "unicode characters counted correctly",
         text:   "日本語テスト",
         maxLen: 3,
         want:   "日本語…",
      },
      {
         name:   "unicode text within limit",
         text:   "日本語",
         maxLen: 3,
         want:   "日本語",
      },
   }

   for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) {
         got := TruncateDescription(tt.text, tt.maxLen)
         if got != tt.want {
            t.Errorf("TruncateDescription(%q, %d) = %q, want %q", tt.text, tt.maxLen, got, tt.want)
         }
      })
   }
}
