package tags

import (
   "encoding/json"
   "errors"
   "fmt"
   "os"
   "path/filepath"
   "regexp"
   "strings"

   "go.uber.org/zap"
)

const (
   tagsFilename   = "tags.json"
   maxTagLength   = 30
   maxDisplayName = 50
   maxGameTags    = 20
)

var tagNameRegex = regexp.MustCompile(`^[a-zA-Z0-9 -]{1,30}$`)

// TagStore represents the persisted tag data.
type TagStore struct {
   LocationDisplayNames map[string]string   `json:"location_display_names"` // scan folder path -> display name
   GameTags             map[string][]string `json:"game_tags"`              // title ID prefix -> list of tag names
   AllTags              []string            `json:"all_tags"`               // master list of all defined tags
}

// TagManager provides operations on the tag store.
type TagManager struct {
   baseFolder string
   store      TagStore
   logger     *zap.SugaredLogger
}

// NewTagManager creates a new TagManager that operates on tags.json in the given baseFolder.
func NewTagManager(baseFolder string) *TagManager {
   return &TagManager{
      baseFolder: baseFolder,
      store: TagStore{
         LocationDisplayNames: make(map[string]string),
         GameTags:             make(map[string][]string),
         AllTags:              []string{},
      },
      logger: zap.S(),
   }
}

// Load reads tags.json from disk. If the file doesn't exist, initializes with an empty store.
// If the file is corrupt or unparseable, logs a warning and initializes empty.
func (tm *TagManager) Load() error {
   filePath := filepath.Join(tm.baseFolder, tagsFilename)

   data, err := os.ReadFile(filePath)
   if err != nil {
      if errors.Is(err, os.ErrNotExist) {
         tm.logger.Infof("Tags file not found, initializing empty tag store")
         tm.initEmptyStore()
         return nil
      }
      return fmt.Errorf("failed to read tags file: %w", err)
   }

   var store TagStore
   if err := json.Unmarshal(data, &store); err != nil {
      tm.logger.Warnf("Tags file is corrupt or unparseable, initializing empty tag store: %v", err)
      tm.initEmptyStore()
      return nil
   }

   // Ensure maps are initialized even if JSON had null values
   if store.LocationDisplayNames == nil {
      store.LocationDisplayNames = make(map[string]string)
   }
   if store.GameTags == nil {
      store.GameTags = make(map[string][]string)
   }
   if store.AllTags == nil {
      store.AllTags = []string{}
   }

   tm.store = store
   return nil
}

// Save writes the current tag store to tags.json with pretty-printed JSON.
func (tm *TagManager) Save() error {
   filePath := filepath.Join(tm.baseFolder, tagsFilename)

   data, err := json.MarshalIndent(tm.store, "", "  ")
   if err != nil {
      return fmt.Errorf("failed to marshal tag store: %w", err)
   }

   if err := os.WriteFile(filePath, data, 0644); err != nil {
      return fmt.Errorf("failed to write tags file: %w", err)
   }

   return nil
}

// ValidateTagName validates a custom tag name.
// Must be 1-30 characters and match the pattern [a-zA-Z0-9 -].
func (tm *TagManager) ValidateTagName(name string) error {
   if len(name) == 0 {
      return errors.New("tag name must not be empty")
   }
   if len(name) > maxTagLength {
      return fmt.Errorf("tag name must be at most %d characters, got %d", maxTagLength, len(name))
   }
   if !tagNameRegex.MatchString(name) {
      return fmt.Errorf("tag name must contain only letters, numbers, spaces, and hyphens")
   }
   return nil
}

// ValidateDisplayName validates a location display name.
// Must be 1-50 characters.
func (tm *TagManager) ValidateDisplayName(name string) error {
   if len(name) == 0 {
      return errors.New("display name must not be empty")
   }
   if len(name) > maxDisplayName {
      return fmt.Errorf("display name must be at most %d characters, got %d", maxDisplayName, len(name))
   }
   return nil
}

// CreateTag adds a new tag to AllTags.
// Validates the name and rejects duplicates (case-insensitive).
func (tm *TagManager) CreateTag(name string) error {
   if err := tm.ValidateTagName(name); err != nil {
      return err
   }

   for _, existing := range tm.store.AllTags {
      if strings.EqualFold(existing, name) {
         return fmt.Errorf("tag %q already exists", existing)
      }
   }

   tm.store.AllTags = append(tm.store.AllTags, name)
   return tm.Save()
}

// AddTagToGame associates a tag with a game.
// The tag must exist in AllTags (case-insensitive lookup, stores original case from AllTags).
// A game cannot have more than 20 tags, and duplicates are rejected.
func (tm *TagManager) AddTagToGame(titleIdPrefix string, tagName string) error {
   // Find the tag in AllTags (case-insensitive), use original case
   canonicalName := ""
   for _, t := range tm.store.AllTags {
      if strings.EqualFold(t, tagName) {
         canonicalName = t
         break
      }
   }
   if canonicalName == "" {
      return fmt.Errorf("tag %q does not exist; create it first", tagName)
   }

   gameTags := tm.store.GameTags[titleIdPrefix]

   // Check for duplicate
   for _, existing := range gameTags {
      if strings.EqualFold(existing, canonicalName) {
         return fmt.Errorf("tag %q is already assigned to game %q", canonicalName, titleIdPrefix)
      }
   }

   // Check max tags limit
   if len(gameTags) >= maxGameTags {
      return fmt.Errorf("game %q already has the maximum of %d tags", titleIdPrefix, maxGameTags)
   }

   tm.store.GameTags[titleIdPrefix] = append(gameTags, canonicalName)
   return tm.Save()
}

// RemoveTagFromGame removes a tag from a game.
// Saves after removing. No error if the tag wasn't on the game.
func (tm *TagManager) RemoveTagFromGame(titleIdPrefix string, tagName string) error {
   gameTags := tm.store.GameTags[titleIdPrefix]
   if len(gameTags) == 0 {
      return tm.Save()
   }

   filtered := make([]string, 0, len(gameTags))
   for _, t := range gameTags {
      if !strings.EqualFold(t, tagName) {
         filtered = append(filtered, t)
      }
   }

   if len(filtered) == 0 {
      delete(tm.store.GameTags, titleIdPrefix)
   } else {
      tm.store.GameTags[titleIdPrefix] = filtered
   }

   return tm.Save()
}

// DeleteTag removes a tag entirely from AllTags and cascades removal from all games.
func (tm *TagManager) DeleteTag(tagName string) error {
   // Remove from AllTags
   filtered := make([]string, 0, len(tm.store.AllTags))
   for _, t := range tm.store.AllTags {
      if !strings.EqualFold(t, tagName) {
         filtered = append(filtered, t)
      }
   }
   tm.store.AllTags = filtered

   // Remove from all games (cascade)
   for titleId, gameTags := range tm.store.GameTags {
      newTags := make([]string, 0, len(gameTags))
      for _, t := range gameTags {
         if !strings.EqualFold(t, tagName) {
            newTags = append(newTags, t)
         }
      }
      if len(newTags) == 0 {
         delete(tm.store.GameTags, titleId)
      } else {
         tm.store.GameTags[titleId] = newTags
      }
   }

   return tm.Save()
}

// GetGameTags returns the tags for a specific game. Returns an empty slice if none.
func (tm *TagManager) GetGameTags(titleIdPrefix string) []string {
   tags := tm.store.GameTags[titleIdPrefix]
   if tags == nil {
      return []string{}
   }
   return tags
}

// GetAllTags returns the master tag list.
func (tm *TagManager) GetAllTags() []string {
   return tm.store.AllTags
}

// GetStore returns the full tag store.
func (tm *TagManager) GetStore() TagStore {
   return tm.store
}

// SetLocationDisplayName sets the display name for a scan folder.
// Validates the display name (1-50 chars) and saves after setting.
func (tm *TagManager) SetLocationDisplayName(scanFolder string, displayName string) error {
   if err := tm.ValidateDisplayName(displayName); err != nil {
      return err
   }

   tm.store.LocationDisplayNames[scanFolder] = displayName
   return tm.Save()
}

// GetLocationTag determines the location tag for a file.
// Finds the deepest (longest path) scan folder that is a parent of filePath.
// If that folder has a display name configured, returns it.
// Otherwise returns the folder's base directory name.
// If no scan folder matches, returns "Unknown".
func (tm *TagManager) GetLocationTag(filePath string, scanFolders []string) string {
   cleanFile := filepath.Clean(filePath)
   bestMatch := ""

   for _, folder := range scanFolders {
      cleanFolder := filepath.Clean(folder)
      // Check if the file path starts with the scan folder path
      if isSubPath(cleanFile, cleanFolder) {
         if len(cleanFolder) > len(bestMatch) {
            bestMatch = cleanFolder
         }
      }
   }

   if bestMatch == "" {
      return "Unknown"
   }

   if displayName, ok := tm.store.LocationDisplayNames[bestMatch]; ok && displayName != "" {
      return displayName
   }

   return filepath.Base(bestMatch)
}

// isSubPath checks if filePath is under the given folder.
func isSubPath(filePath, folder string) bool {
   // Ensure folder path ends with separator for proper prefix matching
   folderWithSep := folder
   if !strings.HasSuffix(folderWithSep, string(filepath.Separator)) {
      folderWithSep += string(filepath.Separator)
   }
   return strings.HasPrefix(filePath, folderWithSep) || filePath == folder
}

// initEmptyStore resets the store to empty defaults.
func (tm *TagManager) initEmptyStore() {
   tm.store = TagStore{
      LocationDisplayNames: make(map[string]string),
      GameTags:             make(map[string][]string),
      AllTags:              []string{},
   }
}
