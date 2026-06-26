package process

import (
   "testing"

   "github.com/trembon/switch-library-manager/db"
)

func TestRecommendKeep_PrefersCompressed(t *testing.T) {
   files := []DuplicateFile{
      {FileName: "game.nsp", Size: 1000, IsCompressed: false},
      {FileName: "game.nsz", Size: 800, IsCompressed: true},
   }

   idx := RecommendKeep(files)
   if idx != 1 {
      t.Errorf("expected index 1 (compressed file), got %d", idx)
   }
}

func TestRecommendKeep_PrefersLargerSameCompression(t *testing.T) {
   files := []DuplicateFile{
      {FileName: "game_small.nsp", Size: 500, IsCompressed: false},
      {FileName: "game_large.nsp", Size: 1000, IsCompressed: false},
   }

   idx := RecommendKeep(files)
   if idx != 1 {
      t.Errorf("expected index 1 (larger file), got %d", idx)
   }
}

func TestRecommendKeep_CompressedOverLargerUncompressed(t *testing.T) {
   files := []DuplicateFile{
      {FileName: "game.xci", Size: 5000, IsCompressed: false},
      {FileName: "game.xcz", Size: 3000, IsCompressed: true},
   }

   idx := RecommendKeep(files)
   if idx != 1 {
      t.Errorf("expected index 1 (compressed even though smaller), got %d", idx)
   }
}

func TestRecommendKeep_LargerCompressedPreferred(t *testing.T) {
   files := []DuplicateFile{
      {FileName: "game_a.nsz", Size: 800, IsCompressed: true},
      {FileName: "game_b.nsz", Size: 900, IsCompressed: true},
   }

   idx := RecommendKeep(files)
   if idx != 1 {
      t.Errorf("expected index 1 (larger compressed file), got %d", idx)
   }
}

func TestRecommendKeep_EmptySlice(t *testing.T) {
   files := []DuplicateFile{}

   idx := RecommendKeep(files)
   if idx != 0 {
      t.Errorf("expected index 0 for empty slice, got %d", idx)
   }
}

func TestRecommendKeep_SingleFile(t *testing.T) {
   files := []DuplicateFile{
      {FileName: "game.nsp", Size: 1000, IsCompressed: false},
   }

   idx := RecommendKeep(files)
   if idx != 0 {
      t.Errorf("expected index 0 for single file, got %d", idx)
   }
}

func TestDetectDuplicates_NoDuplicates(t *testing.T) {
   localDB := &db.LocalSwitchFilesDB{
      TitlesMap: map[string]*db.SwitchGameFiles{
         "0100000000000": {
            BaseExist: true,
            File: db.SwitchFileInfo{
               ExtendedInfo: db.ExtendedFileInfo{
                  FileName:   "Game [0100000000000000][v0].nsp",
                  BaseFolder: "/games/",
                  Size:       1000,
               },
            },
         },
      },
      Skipped: map[db.ExtendedFileInfo]db.SkippedFile{},
   }
   titlesDB := &db.SwitchTitlesDB{
      TitlesMap: map[string]*db.SwitchTitle{},
   }

   result := DetectDuplicates(localDB, titlesDB)

   if result.TotalGroups != 0 {
      t.Errorf("expected 0 groups, got %d", result.TotalGroups)
   }
   if result.ReclaimableSize != 0 {
      t.Errorf("expected 0 reclaimable size, got %d", result.ReclaimableSize)
   }
}

func TestDetectDuplicates_WithDuplicates(t *testing.T) {
   baseFile := db.ExtendedFileInfo{
      FileName:   "Game [0100000000000000][v0].nsp",
      BaseFolder: "/games/",
      Size:       2000,
   }
   dupFile := db.ExtendedFileInfo{
      FileName:   "Game [0100000000000000][v0].nsz",
      BaseFolder: "/games/compressed/",
      Size:       1500,
   }

   localDB := &db.LocalSwitchFilesDB{
      TitlesMap: map[string]*db.SwitchGameFiles{
         "0100000000000": {
            BaseExist: true,
            File: db.SwitchFileInfo{
               ExtendedInfo: baseFile,
            },
            Updates: map[int]db.SwitchFileInfo{},
            Dlc:     map[string]db.SwitchFileInfo{},
         },
      },
      Skipped: map[db.ExtendedFileInfo]db.SkippedFile{
         dupFile: {
            ReasonCode: db.REASON_DUPLICATE,
            ReasonText: "Duplicate base file",
         },
      },
   }
   titlesDB := &db.SwitchTitlesDB{
      TitlesMap: map[string]*db.SwitchTitle{
         "0100000000000": {
            Attributes: db.TitleAttributes{
               Id:   "0100000000000000",
               Name: "Test Game",
            },
         },
      },
   }

   result := DetectDuplicates(localDB, titlesDB)

   if result.TotalGroups != 1 {
      t.Errorf("expected 1 group, got %d", result.TotalGroups)
   }
   if len(result.Groups) != 1 {
      t.Fatalf("expected 1 group, got %d", len(result.Groups))
   }

   group := result.Groups[0]
   if group.TitleName != "Test Game" {
      t.Errorf("expected title name 'Test Game', got '%s'", group.TitleName)
   }
   if len(group.Files) != 2 {
      t.Fatalf("expected 2 files in group, got %d", len(group.Files))
   }

   // The compressed file (nsz) should be recommended
   var recommended *DuplicateFile
   for i := range group.Files {
      if group.Files[i].Recommended {
         recommended = &group.Files[i]
      }
   }
   if recommended == nil {
      t.Fatal("expected one file to be recommended")
   }
   if !recommended.IsCompressed {
      t.Error("expected the compressed file to be recommended")
   }

   // Reclaimable size should be the size of the non-recommended file
   if result.ReclaimableSize != 2000 {
      t.Errorf("expected reclaimable size 2000 (the uncompressed nsp), got %d", result.ReclaimableSize)
   }
}

func TestDetectDuplicates_SkipsNonDuplicateReasons(t *testing.T) {
   baseFile := db.ExtendedFileInfo{
      FileName:   "Game [0100000000000000][v0].nsp",
      BaseFolder: "/games/",
      Size:       2000,
   }
   unsupportedFile := db.ExtendedFileInfo{
      FileName:   "readme.txt",
      BaseFolder: "/games/",
      Size:       100,
   }

   localDB := &db.LocalSwitchFilesDB{
      TitlesMap: map[string]*db.SwitchGameFiles{
         "0100000000000": {
            BaseExist: true,
            File: db.SwitchFileInfo{
               ExtendedInfo: baseFile,
            },
            Updates: map[int]db.SwitchFileInfo{},
            Dlc:     map[string]db.SwitchFileInfo{},
         },
      },
      Skipped: map[db.ExtendedFileInfo]db.SkippedFile{
         unsupportedFile: {
            ReasonCode: db.REASON_UNSUPPORTED_TYPE,
            ReasonText: "Unsupported file type",
         },
      },
   }
   titlesDB := &db.SwitchTitlesDB{
      TitlesMap: map[string]*db.SwitchTitle{},
   }

   result := DetectDuplicates(localDB, titlesDB)

   if result.TotalGroups != 0 {
      t.Errorf("expected 0 groups (non-duplicate reason should be skipped), got %d", result.TotalGroups)
   }
}

func TestToDuplicateFile(t *testing.T) {
   tests := []struct {
      name         string
      info         db.ExtendedFileInfo
      wantType     string
      wantCompress bool
   }{
      {
         name:         "nsp file",
         info:         db.ExtendedFileInfo{FileName: "game.nsp", BaseFolder: "/games/", Size: 1000},
         wantType:     "nsp",
         wantCompress: false,
      },
      {
         name:         "nsz file",
         info:         db.ExtendedFileInfo{FileName: "game.nsz", BaseFolder: "/games/", Size: 800},
         wantType:     "nsz",
         wantCompress: true,
      },
      {
         name:         "xci file",
         info:         db.ExtendedFileInfo{FileName: "game.xci", BaseFolder: "/roms/", Size: 5000},
         wantType:     "xci",
         wantCompress: false,
      },
      {
         name:         "xcz file",
         info:         db.ExtendedFileInfo{FileName: "game.xcz", BaseFolder: "/roms/", Size: 3000},
         wantType:     "xcz",
         wantCompress: true,
      },
   }

   for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) {
         df := toDuplicateFile(tt.info)
         if df.FileType != tt.wantType {
            t.Errorf("expected file type %q, got %q", tt.wantType, df.FileType)
         }
         if df.IsCompressed != tt.wantCompress {
            t.Errorf("expected IsCompressed=%v, got %v", tt.wantCompress, df.IsCompressed)
         }
         if df.Size != tt.info.Size {
            t.Errorf("expected size %d, got %d", tt.info.Size, df.Size)
         }
      })
   }
}

func TestParseTitleIdFromFile(t *testing.T) {
   tests := []struct {
      fileName string
      want     string
   }{
      {"Game [0100000000000000][v0].nsp", "0100000000000000"},
      {"Game [0100ABCdef123456][v65536].nsz", "0100abcdef123456"},
      {"no_brackets.xci", ""},
      {"Game [short].nsp", ""},
      {"Game [01234567890ABCDE][v0].xci", "01234567890abcde"},
   }

   for _, tt := range tests {
      t.Run(tt.fileName, func(t *testing.T) {
         got := parseTitleIdFromFile(tt.fileName)
         if got != tt.want {
            t.Errorf("parseTitleIdFromFile(%q) = %q, want %q", tt.fileName, got, tt.want)
         }
      })
   }
}

func TestTitleIdToPrefix(t *testing.T) {
   tests := []struct {
      titleId string
      want    string
   }{
      {"0100000000000000", "0100000000000"},  // base game (ends in 000)
      {"0100000000000800", "0100000000000"},  // update (ends in 800)
      {"0100000000001001", "0100000000000"},  // DLC: 4th from right '1' -> '0'
   }

   for _, tt := range tests {
      t.Run(tt.titleId, func(t *testing.T) {
         got := titleIdToPrefix(tt.titleId)
         if got != tt.want {
            t.Errorf("titleIdToPrefix(%q) = %q, want %q", tt.titleId, got, tt.want)
         }
      })
   }
}
