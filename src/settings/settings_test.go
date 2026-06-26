package settings

import (
   "strings"
   "testing"
)

func TestValidateSettings_ValidDefaults(t *testing.T) {
   s := &AppSettings{
      GuiPagingSize: 100,
      ArchiveFolder: "",
   }
   if err := ValidateSettings(s); err != nil {
      t.Errorf("expected nil error for valid defaults, got: %v", err)
   }
}

func TestValidateSettings_ValidArchiveFolder(t *testing.T) {
   s := &AppSettings{
      GuiPagingSize: 50,
      ArchiveFolder: "/Users/test/archive",
   }
   if err := ValidateSettings(s); err != nil {
      t.Errorf("expected nil error for valid absolute path, got: %v", err)
   }
}

func TestValidateSettings_RelativeArchiveFolder(t *testing.T) {
   s := &AppSettings{
      GuiPagingSize: 50,
      ArchiveFolder: "relative/path",
   }
   err := ValidateSettings(s)
   if err == nil {
      t.Fatal("expected error for relative archive path, got nil")
   }
   if !strings.Contains(err.Error(), "absolute path") {
      t.Errorf("expected error about absolute path, got: %v", err)
   }
}

func TestValidateSettings_ArchiveFolderTooLong(t *testing.T) {
   longPath := "/" + strings.Repeat("a", 260)
   s := &AppSettings{
      GuiPagingSize: 50,
      ArchiveFolder: longPath,
   }
   err := ValidateSettings(s)
   if err == nil {
      t.Fatal("expected error for path exceeding 260 chars, got nil")
   }
   if !strings.Contains(err.Error(), "260 characters") {
      t.Errorf("expected error about 260 characters, got: %v", err)
   }
}

func TestValidateSettings_ArchiveFolderExactly260(t *testing.T) {
   // 260 chars total: "/" + 259 "a"s
   path := "/" + strings.Repeat("a", 259)
   s := &AppSettings{
      GuiPagingSize: 50,
      ArchiveFolder: path,
   }
   if err := ValidateSettings(s); err != nil {
      t.Errorf("expected nil error for path at exactly 260 chars, got: %v", err)
   }
}

func TestValidateSettings_GuiPagingSizeTooLow(t *testing.T) {
   s := &AppSettings{
      GuiPagingSize: 0,
      ArchiveFolder: "",
   }
   err := ValidateSettings(s)
   if err == nil {
      t.Fatal("expected error for GuiPagingSize=0, got nil")
   }
   if !strings.Contains(err.Error(), "gui_page_size") {
      t.Errorf("expected error about gui_page_size, got: %v", err)
   }
}

func TestValidateSettings_GuiPagingSizeTooHigh(t *testing.T) {
   s := &AppSettings{
      GuiPagingSize: 501,
      ArchiveFolder: "",
   }
   err := ValidateSettings(s)
   if err == nil {
      t.Fatal("expected error for GuiPagingSize=501, got nil")
   }
   if !strings.Contains(err.Error(), "gui_page_size") {
      t.Errorf("expected error about gui_page_size, got: %v", err)
   }
}

func TestValidateSettings_GuiPagingSizeBoundaries(t *testing.T) {
   // Min boundary: 1
   s := &AppSettings{GuiPagingSize: 1, ArchiveFolder: ""}
   if err := ValidateSettings(s); err != nil {
      t.Errorf("expected nil error for GuiPagingSize=1, got: %v", err)
   }

   // Max boundary: 500
   s = &AppSettings{GuiPagingSize: 500, ArchiveFolder: ""}
   if err := ValidateSettings(s); err != nil {
      t.Errorf("expected nil error for GuiPagingSize=500, got: %v", err)
   }
}
