# NX Cellar

Nintendo Switch game library manager — scan, organize, and track updates for NSP/NSZ/XCI/XCZ files.

Fork of [Switch Library Manager](https://github.com/dlorenzo/switch-library-manager) by dlorenzo, originally created by [giwty](https://github.com/giwty/switch-library-manager), with continued improvements by [trembon](https://github.com/trembon/switch-library-manager).

## What's Changed

This fork includes bug fixes and code cleanup on top of v1.12.0:

- Fixed BoltDB read-only transaction bug (bucket creation was silently failing)
- Fixed NCA key revision conversion bug (int-to-string treated as rune)
- Fixed DLC duplicate detection logic (dead condition always evaluated true)
- Replaced deprecated `ioutil` APIs with modern `io`/`os` equivalents
- Added missing error handling throughout settings and database code
- Removed dead code, duplicate CSS blocks, and unused imports
- Optimized regex compilation and repeated settings reads
- Added Apple Silicon (darwin/arm64) build support
- Updated to Electron 13.6.9 and Astilectron 0.57.0

## Features

- Cross platform — Windows, macOS (including Apple Silicon), and Linux
- GUI and command line interfaces
- Scan your local switch backup library (NSP/NSZ/XCI/XCZ)
- Read titleId/version by decrypting NSP/XCI/NSZ (requires prod.keys)
- If no prod.keys present, fallback to read titleId/version by parsing file name (example: `Super Mario Odyssey [0100000000010000][v0].nsp`)
- Lists missing update files (for games and DLC)
- Lists missing DLCs
- Automatically organize games per folder
- Rename files based on metadata
- Delete old update files (keeps only the latest)
- Delete empty folders
- Zero external dependencies for crypto — all implemented in Go

## Keys (optional)

Having a prod.keys file allows the app to read metadata directly from game files. Without it, the app falls back to parsing title ID and version from file names.

The app looks for `prod.keys` in:
1. The app folder
2. `${HOME}/.switch/`
3. A custom path specified in settings.json

Only the `header_key` and `key_area_key_application_XX` keys are required.

## Settings

A `settings.json` file is created on first launch. You can customize folder/file renaming, toggle features, and set title IDs to ignore.

```json
{
 "versions_json_url": "https://raw.githubusercontent.com/blawar/titledb/master/versions.json",
 "titles_json_url": "https://tinfoil.io/repo/db/titles.json",
 "prod_keys": "",
 "folder": "",
 "scan_folders": [],
 "gui": true,
 "check_for_missing_updates": true,
 "check_for_missing_dlc": true,
 "hide_missing_games": false,
 "hide_demo_games": false,
 "organize_options": {
  "create_folder_per_game": false,
  "dlc_folder": "",
  "updates_folder": "",
  "rename_files": false,
  "delete_empty_folders": false,
  "delete_old_update_files": false,
  "folder_name_template": "{TITLE_NAME}",
  "switch_safe_file_names": true,
  "file_name_template": "{TITLE_NAME} ({DLC_NAME})[{TITLE_ID}][v{VERSION}]",
  "process_when_missing_base_game": false,
  "prioritize_compressed": true
 },
 "scan_recursively": true,
 "gui_page_size": 100,
 "ignore_dlc_updates": false,
 "ignore_dlc_title_ids": [],
 "ignore_update_title_ids": [],
 "ignore_file_types": []
}
```

## Naming Template

Supported template variables:

| Variable | Description |
|----------|-------------|
| `{TITLE_NAME}` | Game name |
| `{TITLE_ID}` | Title ID |
| `{VERSION}` | Version number (files only) |
| `{VERSION_TXT}` | Display version like 1.0.0 (files only) |
| `{REGION}` | Region |
| `{TYPE}` | Content type: BASE, UPD, or DLC |
| `{DLC_NAME}` | DLC name (DLC files only) |

## Special File Handling

- **Prevent Renaming:** Add `[nr]` to a filename to lock its name during organization. The file will still be moved to the correct directory.
  - Example: `My Custom Game Name [nr].nsp`
- **Ignore Files:** Prefix with `_` to have the scanner skip it entirely.
  - Example: `_my-temporary-file.zip`

## Usage

### Windows
- Extract the zip file
- Double click `Switch-Library-Manager.exe`
- For CLI mode: set `"gui": false` in settings.json and run from cmd

### macOS
- Extract the zip/app
- You may need to remove quarantine: `xattr -cr Switch-Library-Manager.app`
- Double click the app

### Linux
- Extract the archive
- `chmod +x Switch-Library-Manager`
- Run `./Switch-Library-Manager`

### Console Parameters

| Name | Flag | Value | Description |
|------|------|-------|-------------|
| Mode | -m | console/gui | Override the gui setting |
| NSP Folder | -f | path | Override the folder setting |
| Recursive scan | -r | true/false | Override scan_recursively |
| Export CSV | -e | path | Export missing_updates, missing_dlcs, issues as CSV |

## Building

Requires Go 1.21+ installed.

```bash
cd src
go install github.com/asticode/go-astilectron-bundler/astilectron-bundler@latest
cp $(go env GOPATH)/bin/astilectron-bundler .
./astilectron-bundler
```

Binaries will be in `src/output/` for each platform.

## Credits

- [blawar/titledb](https://github.com/blawar/titledb) — Switch title and version database
- [dlorenzo/switch-library-manager](https://github.com/dlorenzo/switch-library-manager) — Direct upstream fork
- [trembon/switch-library-manager](https://github.com/trembon/switch-library-manager) — Previous maintainer
- [giwty/switch-library-manager](https://github.com/giwty/switch-library-manager) — Original project
- [LibHac](https://github.com/Thealexbarney/LibHac) — Inspiration for Switch file format handling

Developed with assistance from AI tools (Kiro/Claude).

## License

MIT
