package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/asticode/go-astikit"
	"github.com/asticode/go-astilectron"
	bootstrap "github.com/asticode/go-astilectron-bootstrap"
	"github.com/trembon/switch-library-manager/archive"
	"github.com/trembon/switch-library-manager/db"
	"github.com/trembon/switch-library-manager/process"
	"github.com/trembon/switch-library-manager/settings"
	"github.com/trembon/switch-library-manager/tags"
	"go.uber.org/zap"
)

type Pair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  int    `json:"type"`
}

type LocalLibraryData struct {
	LibraryData []LibraryTemplateData `json:"library_data"`
	Issues      []Pair                `json:"issues"`
	NumFiles    int                   `json:"num_files"`
}

type SwitchTitle struct {
	Name        string `json:"name"`
	TitleId     string `json:"titleId"`
	Icon        string `json:"icon"`
	Region      string `json:"region"`
	ReleaseDate string `json:"release_date"`
}

type LibraryTemplateData struct {
	Id          int      `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Dlc         string   `json:"dlc"`
	TitleId     string   `json:"titleId"`
	Path        string   `json:"path"`
	Icon        string   `json:"icon"`
	Update      int      `json:"update"`
	Region      string   `json:"region"`
	Type        string   `json:"type"`
	LocationTag string   `json:"locationTag"`
	CustomTags  []string `json:"customTags"`
}

type ProgressUpdate struct {
	Curr    int    `json:"curr"`
	Total   int    `json:"total"`
	Message string `json:"message"`
}

type State struct {
	sync.Mutex
	switchDB *db.SwitchTitlesDB
	localDB  *db.LocalSwitchFilesDB
	window   *astilectron.Window
}

type Message struct {
	Name    string `json:"name"`
	Payload string `json:"payload"`
}

type GameDetail struct {
	Name          string          `json:"name"`
	TitleId       string          `json:"title_id"`
	Publisher     string          `json:"publisher"`
	Region        string          `json:"region"`
	ReleaseDate   string          `json:"release_date"`
	Description   string          `json:"description"`
	IconUrl       string          `json:"icon_url"`
	BannerUrl     string          `json:"banner_url"`
	LocalVersion  string          `json:"local_version"`
	LatestVersion string          `json:"latest_version"`
	DlcList       []GameDetailDLC `json:"dlc_list"`
	CustomTags    []string        `json:"custom_tags"`
	LocationTag   string          `json:"location_tag"`
}

type GameDetailDLC struct {
	Name    string `json:"name"`
	TitleId string `json:"title_id"`
	Owned   bool   `json:"owned"`
}

type GUI struct {
	state          State
	baseFolder     string
	localDbManager *db.LocalSwitchDBManager
	sugarLogger    *zap.SugaredLogger
	tagManager     *tags.TagManager
	archiveManager *archive.ArchiveManager
}

func CreateGUI(baseFolder string, sugarLogger *zap.SugaredLogger) *GUI {
	return &GUI{state: State{}, baseFolder: baseFolder, sugarLogger: sugarLogger}
}
func (g *GUI) Start() {

	localDbManager, err := db.NewLocalSwitchDBManager(g.baseFolder)
	if err != nil {
		g.sugarLogger.Error("Failed to create local files db\n", err)
		return
	}

	settings.InitSwitchKeys(g.baseFolder)

	g.localDbManager = localDbManager
	defer localDbManager.Close()

	settingsObj := settings.ReadSettings(g.baseFolder)

	// Initialize TagManager
	g.tagManager = tags.NewTagManager(g.baseFolder)
	_ = g.tagManager.Load()

	// Initialize ArchiveManager if archive folder is configured
	if settingsObj.ArchiveFolder != "" {
		am, err := archive.NewArchiveManager(settingsObj.ArchiveFolder, g.sugarLogger)
		if err == nil {
			g.archiveManager = am
		}
	}

	// Run bootstrap
	if err := bootstrap.Run(bootstrap.Options{
		Asset:    Asset,
		AssetDir: AssetDir,
		AstilectronOptions: astilectron.Options{
			AppName:            "Switch Library Manager (" + settings.SLM_VERSION + ")",
			AcceptTCPTimeout:   time.Duration(5) * time.Second,
			AppIconDarwinPath:  "resources/icon.icns",
			AppIconDefaultPath: "resources/icon.png",
			SingleInstance:     true,
		},
		Debug:         false,
		Logger:        log.New(log.Writer(), log.Prefix(), log.Flags()),
		RestoreAssets: RestoreAssets,
		Windows: []*bootstrap.Window{{
			Homepage: "app.html",
			Adapter: func(w *astilectron.Window) {
				g.state.window = w
				g.state.window.OnMessage(g.handleMessage)
			},
			Options: &astilectron.WindowOptions{
				AlwaysOnTop:     astikit.BoolPtr(true),
				BackgroundColor: astikit.StrPtr("#333"),
				Center:          astikit.BoolPtr(true),
				Height:          astikit.IntPtr(settingsObj.WindowHeight),
				Width:           astikit.IntPtr(settingsObj.WindowWidth),
				WebPreferences:  &astilectron.WebPreferences{EnableRemoteModule: astikit.BoolPtr(true)},
			},
		}},
	}); err != nil {
		g.sugarLogger.Error(fmt.Errorf("running bootstrap failed: %w", err))
		log.Fatal(err)
	}
}

func (g *GUI) handleMessage(m *astilectron.EventMessage) interface{} {
	var retValue string
	g.state.Lock()
	defer g.state.Unlock()
	msg := Message{}
	err := m.Unmarshal(&msg)

	if err != nil {
		g.sugarLogger.Error("Failed to parse client message", err)
		return ""
	}

	g.sugarLogger.Debugf("Received message from client [%v]", msg)

	switch msg.Name {
	case "organize":
		g.organizeLibrary()
	case "isKeysFileAvailable":
		keys, _ := settings.SwitchKeys()
		retValue = strconv.FormatBool(keys != nil && keys.GetKey("header_key") != "")
	case "loadSettings":
		retValue = g.loadSettings()

		g.state.window.SetAlwaysOnTop(false)
	case "saveSettings":
		err = g.saveSettings(msg.Payload)
		if err != nil {
			g.sugarLogger.Error(err)
			g.state.window.SendMessage(Message{Name: "error", Payload: err.Error()}, func(m *astilectron.EventMessage) {})
			return ""
		}
	case "missingGames":
		missingGames := g.getMissingGames()
		msg, _ := json.Marshal(missingGames)
		g.state.window.SendMessage(Message{Name: "missingGames", Payload: string(msg)}, func(m *astilectron.EventMessage) {})
	case "checkMaximized":
		settingsObj := settings.ReadSettings(g.baseFolder)
		retValue = strconv.FormatBool(settingsObj.WindowMaximized)
	case "updateLocalLibrary":
		ignoreCache, _ := strconv.ParseBool(msg.Payload)
		localDB, err := g.buildLocalDB(g.localDbManager, ignoreCache)
		if err != nil {
			g.sugarLogger.Error(err)
			g.state.window.SendMessage(Message{Name: "error", Payload: err.Error()}, func(m *astilectron.EventMessage) {})
			return ""
		}
		response := LocalLibraryData{}
		libraryData := []LibraryTemplateData{}
		issues := []Pair{}
		settingsObj := settings.ReadSettings(g.baseFolder)
		scanFolders := append(settingsObj.ScanFolders, settingsObj.Folder)
		for k, v := range localDB.TitlesMap {
			if v.BaseExist {
				version := ""
				name := ""
				if v.File.Metadata.Ncap != nil {
					version = v.File.Metadata.Ncap.DisplayVersion
					name = v.File.Metadata.Ncap.TitleName["AmericanEnglish"].Title
				}

				if v.Updates != nil && len(v.Updates) != 0 {
					if v.Updates[v.LatestUpdate].Metadata.Ncap != nil {
						version = v.Updates[v.LatestUpdate].Metadata.Ncap.DisplayVersion
					} else {
						version = ""
					}
				}
				if title, ok := g.state.switchDB.TitlesMap[k]; ok {
					if title.Attributes.Name != "" {
						name = title.Attributes.Name
					}
					libraryData = append(libraryData,
						LibraryTemplateData{
							Icon:        title.Attributes.IconUrl,
							Name:        name,
							TitleId:     title.Attributes.Id,
							Update:      v.LatestUpdate,
							Version:     version,
							Region:      title.Attributes.Region,
							Type:        getType(v),
							Path:        filepath.Join(v.File.ExtendedInfo.BaseFolder, v.File.ExtendedInfo.FileName),
							LocationTag: g.tagManager.GetLocationTag(filepath.Join(v.File.ExtendedInfo.BaseFolder, v.File.ExtendedInfo.FileName), scanFolders),
							CustomTags:  g.tagManager.GetGameTags(k),
						})
				} else {
					if name == "" {
						name = db.ParseTitleNameFromFileName(v.File.ExtendedInfo.FileName)
					}
					libraryData = append(libraryData,
						LibraryTemplateData{
							Name:        name,
							Update:      v.LatestUpdate,
							Version:     version,
							Type:        getType(v),
							TitleId:     v.File.Metadata.TitleId,
							Path:        v.File.ExtendedInfo.FileName,
							LocationTag: g.tagManager.GetLocationTag(filepath.Join(v.File.ExtendedInfo.BaseFolder, v.File.ExtendedInfo.FileName), scanFolders),
							CustomTags:  g.tagManager.GetGameTags(k),
						})
				}

			} else {
				for _, update := range v.Updates {
					issues = append(issues, Pair{Key: filepath.Join(update.ExtendedInfo.BaseFolder, update.ExtendedInfo.FileName), Value: "Base file is missing", Type: db.REASON_MISSING_BASE})
				}
				for _, dlc := range v.Dlc {
					issues = append(issues, Pair{Key: filepath.Join(dlc.ExtendedInfo.BaseFolder, dlc.ExtendedInfo.FileName), Value: "Base file is missing", Type: db.REASON_MISSING_BASE})
				}
			}
		}
		for k, v := range localDB.Skipped {
			issues = append(issues, Pair{Key: filepath.Join(k.BaseFolder, k.FileName), Value: v.ReasonText, Type: v.ReasonCode})
		}

		response.LibraryData = libraryData
		response.NumFiles = localDB.NumFiles
		response.Issues = issues
		msg, _ := json.Marshal(response)
		g.state.window.SendMessage(Message{Name: "libraryLoaded", Payload: string(msg)}, func(m *astilectron.EventMessage) {})
	case "updateDB":
		if g.state.switchDB == nil {
			switchDb, err := g.buildSwitchDb()
			if err != nil {
				g.sugarLogger.Error(err)
				g.state.window.SendMessage(Message{Name: "error", Payload: err.Error()}, func(m *astilectron.EventMessage) {})
				return ""
			}
			g.state.switchDB = switchDb
		}
	case "hardRescan":
		_ = g.localDbManager.ClearScanData()
		g.state.window.SendMessage(Message{Name: "rescan", Payload: ""}, func(m *astilectron.EventMessage) {})
	case "missingUpdates":
		retValue = g.getMissingUpdates()
	case "missingDlc":
		retValue = g.getMissingDLC()
	case "checkUpdate":
		newUpdate, err := settings.CheckForUpdates()
		if err != nil {
			g.sugarLogger.Error(err)
			if !strings.Contains(err.Error(), "dial tcp") {
				g.state.window.SendMessage(Message{Name: "error", Payload: err.Error()}, func(m *astilectron.EventMessage) {})
			}
		}
		retValue = strconv.FormatBool(newUpdate)
	case "getTagStore":
		storeJson, err := json.Marshal(g.tagManager.GetStore())
		if err != nil {
			g.sugarLogger.Error(err)
			retValue = fmt.Sprintf(`{"error":"%v"}`, err.Error())
		} else {
			retValue = string(storeJson)
		}
	case "saveTags":
		var payload struct {
			Action  string `json:"action"`
			TitleId string `json:"titleId"`
			TagName string `json:"tagName"`
		}
		if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
			g.sugarLogger.Error(err)
			retValue = fmt.Sprintf(`{"error":"%v"}`, err.Error())
		} else {
			var tagErr error
			switch payload.Action {
			case "add":
				tagErr = g.tagManager.AddTagToGame(payload.TitleId, payload.TagName)
			case "remove":
				tagErr = g.tagManager.RemoveTagFromGame(payload.TitleId, payload.TagName)
			case "create":
				tagErr = g.tagManager.CreateTag(payload.TagName)
			case "setLocationName":
				tagErr = g.tagManager.SetLocationDisplayName(payload.TitleId, payload.TagName)
			default:
				tagErr = fmt.Errorf("unknown action: %s", payload.Action)
			}
			if tagErr != nil {
				g.sugarLogger.Error(tagErr)
				retValue = fmt.Sprintf(`{"error":"%v"}`, tagErr.Error())
			} else {
				retValue = `"ok"`
			}
		}
	case "deleteTag":
		if err := g.tagManager.DeleteTag(msg.Payload); err != nil {
			g.sugarLogger.Error(err)
			retValue = fmt.Sprintf(`{"error":"%v"}`, err.Error())
		} else {
			retValue = `"ok"`
		}
	case "addToIgnoreList":
		var payload struct {
			Type    string `json:"type"`
			TitleId string `json:"titleId"`
		}
		if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
			g.sugarLogger.Error(err)
			retValue = fmt.Sprintf(`{"error":"%v"}`, err.Error())
		} else {
			settingsObj := settings.ReadSettings(g.baseFolder)
			alreadyExists := false
			switch payload.Type {
			case "update":
				for _, id := range settingsObj.IgnoreUpdateTitleIds {
					if strings.EqualFold(id, payload.TitleId) {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					settingsObj.IgnoreUpdateTitleIds = append(settingsObj.IgnoreUpdateTitleIds, payload.TitleId)
				}
			case "dlc":
				for _, id := range settingsObj.IgnoreDLCTitleIds {
					if strings.EqualFold(id, payload.TitleId) {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					settingsObj.IgnoreDLCTitleIds = append(settingsObj.IgnoreDLCTitleIds, payload.TitleId)
				}
			}
			settings.SaveSettings(settingsObj, g.baseFolder)
			retJson, _ := json.Marshal(settingsObj)
			retValue = string(retJson)
		}
	case "getGameDetail":
		titleIdPrefix := msg.Payload
		detail := g.buildGameDetail(titleIdPrefix)
		detailJson, _ := json.Marshal(detail)
		retValue = string(detailJson)
	case "getDuplicates":
		result := process.DetectDuplicates(g.state.localDB, g.state.switchDB)
		resultJson, _ := json.Marshal(result)
		retValue = string(resultJson)
	case "resolveDuplicates":
		if g.archiveManager == nil {
			retValue = `{"error":"archive folder not configured"}`
		} else {
			var selections []struct {
				TitleId  string `json:"titleId"`
				KeepPath string `json:"keepPath"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &selections); err != nil {
				g.sugarLogger.Error(err)
				retValue = fmt.Sprintf(`{"error":"%v"}`, err.Error())
			} else {
				type resolveResult struct {
					TitleId  string   `json:"titleId"`
					Archived []string `json:"archived"`
					Errors   []string `json:"errors"`
				}
				var results []resolveResult
				for _, sel := range selections {
					res := resolveResult{TitleId: sel.TitleId}
					// Find all files for this title and archive non-kept ones
					duplicates := process.DetectDuplicates(g.state.localDB, g.state.switchDB)
					for _, group := range duplicates.Groups {
						if group.TitleId == sel.TitleId {
							for _, file := range group.Files {
								if file.Path != sel.KeepPath {
									destPath, archiveErr := g.archiveManager.MoveToArchive(file.Path)
									if archiveErr != nil {
										res.Errors = append(res.Errors, fmt.Sprintf("failed to archive %s: %v", file.Path, archiveErr))
									} else {
										res.Archived = append(res.Archived, destPath)
									}
								}
							}
							break
						}
					}
					results = append(results, res)
				}
				resultJson, _ := json.Marshal(results)
				retValue = string(resultJson)
			}
		}
	}

	g.sugarLogger.Debugf("Server response [%v]", retValue)

	return retValue
}

func getType(gameFile *db.SwitchGameFiles) string {
	if gameFile.IsSplit {
		return "split"
	}
	if gameFile.MultiContent {
		return "multi-content"
	}
	ext := filepath.Ext(gameFile.File.ExtendedInfo.FileName)
	if len(ext) > 1 {
		return ext[1:]
	}
	return ""
}

func (g *GUI) saveSettings(settingsJson string) error {
	s := settings.AppSettings{}
	err := json.Unmarshal([]byte(settingsJson), &s)
	if err != nil {
		return err
	}
	settings.SaveSettings(&s, g.baseFolder)
	return nil
}

func (g *GUI) getMissingDLC() string {
	settingsObj := settings.ReadSettings(g.baseFolder)
	ignoreIds := map[string]struct{}{}
	for _, id := range settingsObj.IgnoreDLCTitleIds {
		ignoreIds[strings.ToLower(id)] = struct{}{}
	}
	missingDLC := process.ScanForMissingDLC(g.state.localDB.TitlesMap, g.state.switchDB.TitlesMap, ignoreIds)
	values := make([]process.IncompleteTitle, len(missingDLC))
	i := 0
	for _, missingUpdate := range missingDLC {
		values[i] = missingUpdate
		i++
	}

	msg, _ := json.Marshal(values)
	return string(msg)
}

func (g *GUI) getMissingUpdates() string {
	settingsObj := settings.ReadSettings(g.baseFolder)
	ignoreIds := map[string]struct{}{}
	for _, id := range settingsObj.IgnoreUpdateTitleIds {
		ignoreIds[strings.ToLower(id)] = struct{}{}
	}
	missingUpdates := process.ScanForMissingUpdates(g.state.localDB.TitlesMap, g.state.switchDB.TitlesMap, ignoreIds, settingsObj.IgnoreDLCUpdates)
	values := make([]process.IncompleteTitle, len(missingUpdates))
	i := 0
	for _, missingUpdate := range missingUpdates {
		values[i] = missingUpdate
		i++
	}

	msg, _ := json.Marshal(values)
	return string(msg)
}

func (g *GUI) loadSettings() string {
	return settings.ReadSettingsAsJSON(g.baseFolder)
}

func (g *GUI) buildSwitchDb() (*db.SwitchTitlesDB, error) {
	settingsObj := settings.ReadSettings(g.baseFolder)
	//1. load the titles JSON object
	g.UpdateProgress(1, 4, "Downloading titles.json")
	filename := filepath.Join(g.baseFolder, settings.TITLE_JSON_FILENAME)
	titleFile, titlesEtag, err := db.LoadAndUpdateFile(settingsObj.TitlesJsonUrl, filename, settingsObj.TitlesEtag)
	if err != nil {
		return nil, errors.New("failed to download switch titles [reason:" + err.Error() + "]")
	}
	settingsObj.TitlesEtag = titlesEtag

	g.UpdateProgress(2, 4, "Downloading versions.json")
	filename = filepath.Join(g.baseFolder, settings.VERSIONS_JSON_FILENAME)
	versionsFile, versionsEtag, err := db.LoadAndUpdateFile(settingsObj.VersionsJsonUrl, filename, settingsObj.VersionsEtag)
	if err != nil {
		return nil, errors.New("failed to download switch updates [reason:" + err.Error() + "]")
	}
	settingsObj.VersionsEtag = versionsEtag

	settings.SaveSettings(settingsObj, g.baseFolder)

	g.UpdateProgress(3, 4, "Processing switch titles and updates")
	switchTitleDB, err := db.CreateSwitchTitleDB(titleFile, versionsFile)
	g.UpdateProgress(4, 4, "Finishing up...")
	return switchTitleDB, err
}

func (g *GUI) buildLocalDB(localDbManager *db.LocalSwitchDBManager, ignoreCache bool) (*db.LocalSwitchFilesDB, error) {
	settingsObj := settings.ReadSettings(g.baseFolder)
	folderToScan := settingsObj.Folder
	recursiveMode := settingsObj.ScanRecursively

	scanFolders := settingsObj.ScanFolders
	scanFolders = append(scanFolders, folderToScan)
	localDB, err := localDbManager.CreateLocalSwitchFilesDB(scanFolders, g, recursiveMode, ignoreCache)
	g.state.localDB = localDB
	return localDB, err
}

func (g *GUI) organizeLibrary() {
	settingsObj := settings.ReadSettings(g.baseFolder)
	folderToScan := settingsObj.Folder
	options := settingsObj.OrganizeOptions
	if !process.IsOptionsValid(options) {
		zap.S().Error("the organize options in settings.json are not valid, please check that the template contains file/folder name")
		g.state.window.SendMessage(Message{Name: "error", Payload: "the organize options in settings.json are not valid, please check that the template contains file/folder name"}, func(m *astilectron.EventMessage) {})
		return
	}
	if settingsObj.OrganizeOptions.DeleteOldUpdateFiles {
		var archiveMgr *archive.ArchiveManager
		if settingsObj.ArchiveFolder != "" {
			var err error
			archiveMgr, err = archive.NewArchiveManager(settingsObj.ArchiveFolder, g.sugarLogger)
			if err != nil {
				g.sugarLogger.Warnf("Failed to create archive manager, falling back to delete: %v", err)
			} else {
				if err := archiveMgr.EnsureArchiveDir(); err != nil {
					g.sugarLogger.Warnf("Failed to create archive directory, falling back to delete: %v", err)
					archiveMgr = nil
				}
			}
		}
		process.DeleteOldUpdates(g.baseFolder, g.state.localDB, g, archiveMgr)
	}
	process.OrganizeByFolders(folderToScan, g.state.localDB, g.state.switchDB, g)
}

func (g *GUI) UpdateProgress(curr int, total int, message string) {
	progressMessage := ProgressUpdate{curr, total, message}
	g.sugarLogger.Debugf("%v (%v/%v)", message, curr, total)
	msg, err := json.Marshal(progressMessage)
	if err != nil {
		g.sugarLogger.Error(err)
		return
	}

	g.state.window.SendMessage(Message{Name: "updateProgress", Payload: string(msg)}, func(m *astilectron.EventMessage) {})
}

func (g *GUI) getMissingGames() []SwitchTitle {
	var result []SwitchTitle
	options := settings.ReadSettings(g.baseFolder)
	for k, v := range g.state.switchDB.TitlesMap {
		if _, ok := g.state.localDB.TitlesMap[k]; ok {
			continue
		}
		if v.Attributes.Name == "" || v.Attributes.Id == "" {
			continue
		}

		if options.HideDemoGames && v.Attributes.IsDemo {
			continue
		}

		result = append(result, SwitchTitle{
			TitleId:     v.Attributes.Id,
			Name:        v.Attributes.Name,
			Icon:        v.Attributes.BannerUrl,
			Region:      v.Attributes.Region,
			ReleaseDate: v.Attributes.ParsedReleaseDate,
		})
	}
	return result

}

// buildGameDetail constructs a GameDetail struct from switchDB and localDB data.
func (g *GUI) buildGameDetail(titleIdPrefix string) GameDetail {
	detail := GameDetail{
		TitleId:    titleIdPrefix,
		DlcList:    []GameDetailDLC{},
		CustomTags: []string{},
	}

	// Look up title metadata from switchDB
	if g.state.switchDB != nil {
		if title, ok := g.state.switchDB.TitlesMap[titleIdPrefix]; ok {
			detail.Name = title.Attributes.Name
			detail.Publisher = title.Attributes.Publisher
			detail.Region = title.Attributes.Region
			detail.ReleaseDate = title.Attributes.ParsedReleaseDate
			detail.Description = db.TruncateDescription(title.Attributes.Description, 1000)
			detail.IconUrl = title.Attributes.IconUrl
			detail.BannerUrl = title.Attributes.BannerUrl

			// Get latest version from switchDB updates
			if title.Updates != nil {
				latestVer := 0
				for ver := range title.Updates {
					if ver > latestVer {
						latestVer = ver
					}
				}
				if latestVer > 0 {
					detail.LatestVersion = strconv.Itoa(latestVer)
				}
			}

			// Build DLC list from switchDB, check ownership against localDB
			for dlcId, dlcAttr := range title.Dlc {
				owned := false
				if g.state.localDB != nil {
					// Check if the DLC title ID prefix exists in localDB
					// DLC IDs need to be checked as-is since they're stored by prefix in localDB
					if _, exists := g.state.localDB.TitlesMap[titleIdPrefix]; exists {
						localGame := g.state.localDB.TitlesMap[titleIdPrefix]
						if _, dlcOwned := localGame.Dlc[dlcId]; dlcOwned {
							owned = true
						}
					}
				}
				dlcName := dlcAttr.Name
				if dlcName == "" {
					dlcName = dlcId
				}
				detail.DlcList = append(detail.DlcList, GameDetailDLC{
					Name:    dlcName,
					TitleId: dlcId,
					Owned:   owned,
				})
			}
		}
	}

	// Get local version from localDB
	if g.state.localDB != nil {
		if gameFiles, ok := g.state.localDB.TitlesMap[titleIdPrefix]; ok {
			if gameFiles.LatestUpdate > 0 {
				detail.LocalVersion = strconv.Itoa(gameFiles.LatestUpdate)
			}
			// Fallback name from local file if switchDB didn't have one
			if detail.Name == "" {
				if gameFiles.File.Metadata != nil && gameFiles.File.Metadata.Ncap != nil {
					detail.Name = gameFiles.File.Metadata.Ncap.TitleName["AmericanEnglish"].Title
				}
				if detail.Name == "" {
					detail.Name = db.ParseTitleNameFromFileName(gameFiles.File.ExtendedInfo.FileName)
				}
			}

			// Get location tag from file path
			filePath := filepath.Join(gameFiles.File.ExtendedInfo.BaseFolder, gameFiles.File.ExtendedInfo.FileName)
			settingsObj := settings.ReadSettings(g.baseFolder)
			scanFolders := settingsObj.ScanFolders
			scanFolders = append(scanFolders, settingsObj.Folder)
			detail.LocationTag = g.tagManager.GetLocationTag(filePath, scanFolders)
		}
	}

	// Get custom tags
	detail.CustomTags = g.tagManager.GetGameTags(titleIdPrefix)

	return detail
}
