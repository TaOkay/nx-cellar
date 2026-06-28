const { shell, dialog } = require('electron').remote

$(function () {

    let state = {
        settings:{},
        keys:false
    };

    let currTable

    // Dark mode helper function
    function setDarkMode(isDark) {
        if (isDark) {
            document.body.classList.add("bootstrap-dark");
            document.body.classList.remove("bootstrap");
            try { require('electron').remote.nativeTheme.themeSource = 'dark'; } catch(e){}
            $('meta[name="color-scheme"]').attr("content", "dark");
            $("#toggle-dark-mode").text("☀️");
        } else {
            document.body.classList.add("bootstrap");
            document.body.classList.remove("bootstrap-dark");
            try { require('electron').remote.nativeTheme.themeSource = 'light'; } catch(e){}
            $('meta[name="color-scheme"]').attr("content", "light");
            $("#toggle-dark-mode").text("🌙");
        }
    }

    // Fluent UI formatter for Title + Thumbnail
    const fluentTitleFormatter = function(cell, formatterParams, onRendered){
        const data = cell.getRow().getData();
        const imgSrc = data.icon || (data.Attributes && data.Attributes.bannerUrl) || null;
        const title = data.name || (data.Attributes && data.Attributes.name) || 'Unknown Title';
        
        if (imgSrc) {
            return `<div style="display:flex; align-items:center; gap: 12px; padding: 4px 0;">
                      <img src="${imgSrc}" style="width: 52px; height: 52px; border-radius: 6px; object-fit: cover; box-shadow: 0 2px 6px rgba(0,0,0,0.15);">
                      <div style="font-weight: 600; font-size: 14px; white-space: normal; line-height: 1.3; color: var(--fluent-text, inherit);">${title}</div>
                    </div>`;
        } else {
            return `<div style="font-weight: 600; font-size: 14px; white-space: normal; padding: 4px 0; color: var(--fluent-text, inherit);">${title}</div>`;
        }
    };

    // Fluent UI formatter for File paths
    const fluentFileFormatter = function(cell, formatterParams, onRendered){
        const fullPath = cell.getValue();
        if (!fullPath) return "";

        // Try to split the path by standard slashes to separate filename from directory
        const normalizedPath = fullPath.replace(/\\/g, '/');
        const parts = normalizedPath.split('/');
        
        const fileName = parts.pop();
        const dirName = parts.join('/') || "/";
        
        return `<div style="display:flex; flex-direction:column; justify-content:center; padding: 4px 0; cursor: pointer;">
                  <div style="font-weight: 500; font-size: 13px; color: var(--fluent-text, inherit); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%;" title="${fileName}">${fileName}</div>
                  <div style="font-weight: 400; font-size: 11px; color: #888; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%;" title="${dirName}">${dirName}</div>
                </div>`;
    };

    //handle tabs action
    $('.tabgroup > div').hide();
    // loadTab($('.tabgroup > div:first-of-type'));

    /**
     * ColumnConfigManager - Manages per-tab column configuration with localStorage persistence.
     * Handles column visibility, width, and order for Tabulator grids.
     */
    class ColumnConfigManager {
        /**
         * @param {string} tabId - Unique identifier for the tab (e.g., 'library', 'updates', 'dlc', 'status', 'missing')
         * @param {Array} defaultColumns - Array of Tabulator column definition objects (the default columns for this tab)
         */
        constructor(tabId, defaultColumns) {
            this.tabId = tabId;
            this.defaultColumns = defaultColumns;
            this.storageKey = `nxcellar_columns_${tabId}`;
            this.version = 1;
        }

        /**
         * Save current column config to localStorage.
         * Key: `nxcellar_columns_${tabId}`
         * Value: JSON with { version: 1, columns: [{field, visible, width, position}] }
         */
        save() {
            const config = {
                version: this.version,
                columns: this.defaultColumns.map((col, index) => ({
                    field: col.field,
                    visible: col.visible !== false,
                    width: col.width || null,
                    position: index
                }))
            };

            // Apply any overrides from the internal state
            const saved = this.load();
            if (saved) {
                config.columns = saved.columns;
            }

            localStorage.setItem(this.storageKey, JSON.stringify(config));
        }

        /**
         * Load column config from localStorage.
         * Returns the saved config or null if not found/corrupt.
         * Validates version field. If version doesn't match or data is corrupt, returns null.
         */
        load() {
            try {
                const raw = localStorage.getItem(this.storageKey);
                if (!raw) return null;

                const config = JSON.parse(raw);

                if (!config || config.version !== this.version) return null;
                if (!Array.isArray(config.columns)) return null;

                return config;
            } catch (e) {
                return null;
            }
        }

        /**
         * Reset — remove localStorage key, return default columns.
         * @returns {Array} - The default Tabulator column definitions
         */
        reset() {
            localStorage.removeItem(this.storageKey);
            return this.defaultColumns;
        }

        /**
         * Set column visibility. Enforces at-least-one-visible invariant.
         * @param {string} field - Column field name
         * @param {boolean} visible - Whether to show or hide
         * @returns {boolean} - false if operation rejected (would hide last column)
         */
        setVisibility(field, visible) {
            const config = this._getWorkingConfig();

            // If hiding, check that at least one other column remains visible
            if (!visible) {
                const visibleCount = config.columns.filter(c => c.visible).length;
                if (visibleCount <= 1) {
                    // Check if the column we're hiding is the last visible one
                    const target = config.columns.find(c => c.field === field);
                    if (target && target.visible) {
                        return false;
                    }
                }
            }

            const col = config.columns.find(c => c.field === field);
            if (col) {
                col.visible = visible;
            }

            this._saveConfig(config);
            return true;
        }

        /**
         * Set column order from an array of field names.
         * @param {Array<string>} fields - Ordered field names
         */
        setOrder(fields) {
            const config = this._getWorkingConfig();

            fields.forEach((field, index) => {
                const col = config.columns.find(c => c.field === field);
                if (col) {
                    col.position = index;
                }
            });

            this._saveConfig(config);
        }

        /**
         * Set column width. Clamps to [50, viewportWidth].
         * @param {string} field - Column field name
         * @param {number} width - Desired width in pixels
         */
        setWidth(field, width) {
            const config = this._getWorkingConfig();
            const minWidth = 50;
            const maxWidth = window.innerWidth;
            const clampedWidth = Math.max(minWidth, Math.min(maxWidth, width));

            const col = config.columns.find(c => c.field === field);
            if (col) {
                col.width = clampedWidth;
            }

            this._saveConfig(config);
        }

        /**
         * Get Tabulator column definitions with saved config applied.
         * Merges saved visibility/width/order with the default column definitions.
         * @returns {Array} - Tabulator column definition objects ready for use
         */
        getColumns() {
            const config = this.load();

            if (!config) {
                // No saved config, return defaults with visible columns only
                return this.defaultColumns.filter(col => col.visible !== false);
            }

            // Build a map of saved settings keyed by field
            const savedMap = {};
            config.columns.forEach(col => {
                savedMap[col.field] = col;
            });

            // Merge defaults with saved config
            const merged = this.defaultColumns.map((defaultCol, index) => {
                const saved = savedMap[defaultCol.field];
                if (saved) {
                    const col = Object.assign({}, defaultCol);
                    col.visible = saved.visible;
                    if (saved.width != null) {
                        col.width = saved.width;
                    }
                    col._position = saved.position != null ? saved.position : index;
                    return col;
                } else {
                    // New column not in saved config — include with defaults
                    const col = Object.assign({}, defaultCol);
                    col._position = index + 1000; // Place new columns at the end
                    return col;
                }
            });

            // Sort by position
            merged.sort((a, b) => a._position - b._position);

            // Clean up internal _position property and filter to visible only
            return merged
                .filter(col => col.visible !== false)
                .map(col => {
                    const cleaned = Object.assign({}, col);
                    delete cleaned._position;
                    return cleaned;
                });
        }

        /**
         * Get context menu items for column visibility toggles.
         * Returns array of {label, field, visible, disabled} objects.
         * Includes a "Reset Columns" item at the end.
         * @returns {Array}
         */
        getContextMenuItems() {
            const config = this._getWorkingConfig();
            const visibleCount = config.columns.filter(c => c.visible).length;

            const items = config.columns.map(col => {
                // Find the default column to get its title
                const defaultCol = this.defaultColumns.find(d => d.field === col.field);
                const label = (defaultCol && defaultCol.title) || col.field;

                return {
                    label: label,
                    field: col.field,
                    visible: col.visible,
                    disabled: col.visible && visibleCount <= 1
                };
            });

            // Add reset option at the end
            items.push({
                label: "Reset Columns",
                field: null,
                visible: null,
                disabled: false
            });

            return items;
        }

        /**
         * Get the working config — loads from localStorage or builds from defaults.
         * @private
         * @returns {Object} - Config object with version and columns array
         */
        _getWorkingConfig() {
            const saved = this.load();
            if (saved) {
                // Filter out columns that no longer exist in defaults
                const defaultFields = new Set(this.defaultColumns.map(c => c.field));
                saved.columns = saved.columns.filter(c => defaultFields.has(c.field));

                // Add any new default columns not in saved config
                const savedFields = new Set(saved.columns.map(c => c.field));
                this.defaultColumns.forEach((defaultCol, index) => {
                    if (!savedFields.has(defaultCol.field)) {
                        saved.columns.push({
                            field: defaultCol.field,
                            visible: defaultCol.visible !== false,
                            width: defaultCol.width || null,
                            position: saved.columns.length
                        });
                    }
                });

                return saved;
            }

            // Build from defaults
            return {
                version: this.version,
                columns: this.defaultColumns.map((col, index) => ({
                    field: col.field,
                    visible: col.visible !== false,
                    width: col.width || null,
                    position: index
                }))
            };
        }

        /**
         * Save the given config object to localStorage.
         * @private
         * @param {Object} config - Config object to persist
         */
        _saveConfig(config) {
            localStorage.setItem(this.storageKey, JSON.stringify(config));
        }
    }

    /**
     * ContextMenuManager - Manages right-click context menus for the application.
     * Handles column visibility menus and action menus (ignore title/DLC).
     */
    class ContextMenuManager {
        constructor() {
            this.menuElement = null;
            this._boundHide = this.hide.bind(this);
        }

        /**
         * Show a context menu at the given position with the specified items.
         * @param {number} x - X coordinate (page position)
         * @param {number} y - Y coordinate (page position)
         * @param {Array} items - Menu items. Each item has:
         *   - {string} label - Display text
         *   - {string|null} field - Field identifier (null for actions like "Reset")
         *   - {boolean|null} visible - Checkbox state (null for non-checkbox items)
         *   - {boolean} disabled - Whether the item is disabled
         *   - {string} [type] - 'separator' for divider, 'action' for non-checkbox items
         *   - {function} [onClick] - Callback when clicked
         */
        show(x, y, items) {
            this.hide(); // Remove any existing menu

            const menu = document.createElement('div');
            menu.className = 'context-menu';

            items.forEach(item => {
                if (item.type === 'separator') {
                    const sep = document.createElement('div');
                    sep.className = 'context-menu-separator';
                    menu.appendChild(sep);
                    return;
                }

                const menuItem = document.createElement('div');
                menuItem.className = 'context-menu-item' + (item.disabled ? ' disabled' : '');

                if (item.visible !== null && item.visible !== undefined) {
                    // Checkbox item (for column visibility)
                    const checkbox = document.createElement('input');
                    checkbox.type = 'checkbox';
                    checkbox.checked = item.visible;
                    checkbox.disabled = item.disabled;
                    menuItem.appendChild(checkbox);
                }

                const label = document.createElement('span');
                label.textContent = item.label;
                menuItem.appendChild(label);

                if (!item.disabled && item.onClick) {
                    menuItem.addEventListener('click', (e) => {
                        e.stopPropagation();
                        item.onClick(item);
                        this.hide();
                    });
                }

                menu.appendChild(menuItem);
            });

            // Position the menu, ensuring it stays within viewport
            menu.style.left = x + 'px';
            menu.style.top = y + 'px';
            document.body.appendChild(menu);

            // Adjust if menu goes off-screen
            const rect = menu.getBoundingClientRect();
            if (rect.right > window.innerWidth) {
                menu.style.left = (x - rect.width) + 'px';
            }
            if (rect.bottom > window.innerHeight) {
                menu.style.top = (y - rect.height) + 'px';
            }

            this.menuElement = menu;

            // Close on click outside or Escape
            setTimeout(() => {
                document.addEventListener('click', this._boundHide);
                document.addEventListener('keydown', this._handleKeydown.bind(this));
            }, 0);
        }

        /**
         * Hide/dismiss the context menu.
         */
        hide() {
            if (this.menuElement) {
                this.menuElement.remove();
                this.menuElement = null;
            }
            document.removeEventListener('click', this._boundHide);
            document.removeEventListener('keydown', this._handleKeydown);
        }

        /**
         * Handle keydown for Escape key dismissal.
         * @private
         */
        _handleKeydown(e) {
            if (e.key === 'Escape') {
                this.hide();
            }
        }
    }

    // This will wait for the astilectron namespace to be ready
    document.addEventListener('astilectron-ready', function () {
        
        // Restore Maximized State from backend settings
        astilectron.sendMessage({name: "checkMaximized", payload: ""}, function(message) {
            if (message === "true") {
                try { require('electron').remote.getCurrentWindow().maximize(); } catch(e){}
            }
        });

        let sendMessage = function (name, payload, callback) {
            astilectron.sendMessage({name: name, payload: payload}, callback)
        };

        const contextMenu = new ContextMenuManager();

        sendMessage("loadSettings", "", function (message) {
            state.settings = JSON.parse(message);

            if(state.settings.hide_missing_games){
                document.getElementById("tab_btns").classList.add("hide_missing_games");
            }

            // Apply Dark Mode from settings
            setDarkMode(state.settings.dark_mode);
        });

        sendMessage("isKeysFileAvailable", "", function (message) {
            state.keys = message
        });

        sendMessage("checkUpdate", "", function (message) {
            if (message === "false"){
                return
            }
            dialog.showMessageBox(null, {
                type: 'info',
                buttons: ['Ok'],
                defaultId: 0,
                title: 'New update available',
                message: 'There is a new update available, please download from Github',
                detail: message.payload
            });
        });

        $(".progress-container").show();
        $(".progress-type").text("Downloading latest Switch titles/versions ...");

        sendMessage("updateDB", "", function (message) {
            scanLocalFolder();
        });

        astilectron.onMessage(function (message) {
            // Process message
            // console.log(message)
            let pcg = 0
            if (message.name === "updateProgress") {
                let pp = JSON.parse(message.payload);
                let count = pp.curr;
                let total = pp.total;
                $('.progress-msg').text(pp.message);
                if (count !== -1 && total !== -1){
                    pcg = Math.floor(count / total * 100);
                    $('.progress-bar').attr('aria-valuenow', pcg);
                    $('.progress-bar').attr('style', 'width:' + Number(pcg) + '%');
                    $('.progress-bar').text(pcg + "%");
                }
                if (pcg === 100){
                    $(".progress-container").hide();
                }else{
                    $(".progress-container").show();
                }
            }
            else if (message.name === "libraryLoaded") {
                state.library = JSON.parse(message.payload);
                loadTab("#library")
            }
            else if (message.name === "missingGames") {
                state.missingGames = JSON.parse(message.payload);
                loadTab("#missing")
            }
            else if (message.name === "error") {
                dialog.showMessageBox(null, {
                    type: 'error',
                    buttons: ['Ok'],
                    defaultId: 0,
                    title: 'Error',
                    message: 'An unexpected error occurred',
                    detail: message.payload
                });
                state.settings.folder = undefined;
                $(".progress-container").hide();
                loadTab("#library")
            }
            else if (message.name === "rescan") {
                state.library = undefined;
                state.updates = undefined;
                state.dlc = undefined;
                scanLocalFolder(true)
            }
        });

        let openFolderPicker = function (mode) {
            //show info
            dialog.showOpenDialog({
                properties: ['openDirectory'],
                message:"Select games folder"
            }).then(partial(updateFolder,mode))
                .catch(error => console.log(error))
        };

        let scanLocalFolder = function(mode){
            if (!state.settings.folder){
                loadTab("#library")
                return
            }
            //show progress
            $(".progress-container").show();
            $(".progress-type").text("Scanning local library...");

            sendMessage("updateLocalLibrary", ""+mode, (r => {}))
        };

        let updateFolder = function (mode,result) {
            if (result.canceled) {
                console.log("user aborted");
                return
            }
            if (!result.filePaths || !result.filePaths.length){
                return
            }

            if (mode === "add"){
                state.settings.scan_folders = state.settings.scan_folders || []
                if (!state.settings.scan_folders.includes(result.filePaths[0])){
                    state.settings.scan_folders.push(result.filePaths[0]);
                }else{
                    return;
                }

            }else{
                state.settings.folder = result.filePaths[0];
            }
            $('.tabgroup > div').hide();
            console.log("selected folder:"+result.filePaths[0]);
            state.library = undefined;
            state.updates = undefined;
            state.dlc = undefined;
            sendMessage("saveSettings", JSON.stringify(state.settings), scanLocalFolder);
        };


        function loadTab(target) {
            hideCurrentTab();

            $("#tab_btns a[href='" + target + "']").addClass('active');
            $(target).show();

            if (target === "#settings") {
                let settingsHtml = $(target + "Template").render({
                    settings: state.settings,
                    ignore_update_title_ids_str: state.settings.ignore_update_title_ids ? state.settings.ignore_update_title_ids.join('\n') : "",
                    ignore_dlc_title_ids_str: state.settings.ignore_dlc_title_ids ? state.settings.ignore_dlc_title_ids.join('\n') : ""
                });
                $(target).html(settingsHtml);
            } else if (target === "#organize") {
                let html = $(target + "Template").render({folder: state.settings.folder,settings:state.settings})
                $(target).html(html);
            } else if (target === "#updates") {
                if (state.settings.folder && !state.library){
                    return
                }
                if (state.library && !state.updates){
                    sendMessage("missingUpdates", "", (r => {
                        state.updates = JSON.parse(r)
                        loadTab("#updates")
                    }));
                    return
                }
                let html = $(target + "Template").render({folder: state.settings.folder,updates:state.updates})
                $(target).html(html);
                if (state.updates && state.updates.length) {
                    currTable = new Tabulator("#updates-table", {
                        layout:"fitDataStretch",
                        headerSortTristate: true,
                        initialSort:[
                            {column:"latest_update_date", dir:"desc"}, //sort by this first
                        ],
                        pagination: "local",
                        paginationSize: state.settings.gui_page_size,
                        data: state.updates,
                        columns: [
                            {title: "Game", field: "Attributes.name", headerFilter:"input", formatter:fluentTitleFormatter, width:400},
                            {title: "Type", field: "Meta.type", headerFilter:"input"},
                            {title: "Title ID", field: "Attributes.id", hozAlign: "right", sorter: "number"},
                            {title: "Local version", field: "local_update", hozAlign: "right", sorter: "number"},
                            {title: "Available version", field: "latest_update", hozAlign: "right"},
                            {title: "Update date", field: "latest_update_date",sorter:"date", sorterParams:{format:"YYYY-MM-DD"}}
                        ],
                    });
                    currTable.on("rowContext", function(e, row){
                        e.preventDefault();
                        const data = row.getData();
                        const titleId = data.Attributes ? data.Attributes.id : "";
                        const name = data.Attributes ? data.Attributes.name : "Unknown";
                        contextMenu.show(e.pageX, e.pageY, [
                            {label: "Ignore \"" + name + "\"", field: titleId, visible: null, disabled: !titleId, onClick: function(item) {
                                sendMessage("addToIgnoreList", JSON.stringify({type: "update", titleId: item.field}), function(r) {
                                    row.delete();
                                });
                            }}
                        ]);
                    });
                }
            } else if (target === "#dlc") {
                if (state.settings.folder && !state.library){
                    return
                }
                if (state.library && !state.dlc){
                    sendMessage("missingDlc", "", (r => {
                        state.dlc = JSON.parse(r)
                        loadTab("#dlc")
                    }));
                    return
                }
                let html = $(target + "Template").render({folder: state.settings.folder,dlc:state.dlc});
                $(target).html(html);
                if (state.dlc && state.dlc.length) {
                    currTable = new Tabulator("#dlc-table", {
                        layout:"fitDataStretch",
                        headerSortTristate: true,
                        initialSort:[
                            {column:"Attributes.name", dir:"asc"}, //sort by this first
                        ],
                        pagination: "local",
                        paginationSize: state.settings.gui_page_size,
                        data: state.dlc,
                        columns: [
                            {title: "Game", field: "Attributes.name", headerFilter:"input",formatter:fluentTitleFormatter, width:400},
                            {title: "# Missing", field: "missing_dlc.length"},
                            {title: "Missing DLC", field: "missing_dlc",formatter:function(cell, formatterParams, onRendered){
                                    let value = ""
                                    for (var i in cell.getValue())
                                    {
                                        value +="<div>"+cell.getValue()[i]+"</div>"
                                    }
                                    return value
                                }}
                        ],
                    });
                    currTable.on("rowContext", function(e, row){
                        e.preventDefault();
                        const data = row.getData();
                        const titleId = data.Attributes ? data.Attributes.id : "";
                        const name = data.Attributes ? data.Attributes.name : "Unknown";
                        contextMenu.show(e.pageX, e.pageY, [
                            {label: "Ignore \"" + name + "\"", field: titleId, visible: null, disabled: !titleId, onClick: function(item) {
                                sendMessage("addToIgnoreList", JSON.stringify({type: "dlc", titleId: item.field}), function(r) {
                                    row.delete();
                                });
                            }}
                        ]);
                    });
                }
            } else if (target === "#status") {
                if (state.settings.folder && !state.library){
                    return
                }
                let html = $(target + "Template").render({folder: state.settings.folder,library:state.library ? state.library.issues: undefined,numFiles:state.library ? state.library.num_files:-1});
                $(target).html(html);
                if (state.library.issues && state.library.issues.length) {
                    currTable = new Tabulator("#status-table", {
                        layout:"fitDataStretch",
                        headerSortTristate: true,
                        pagination: "local",
                        paginationSize: state.settings.gui_page_size,
                        data: state.library.issues,
                        initialSort:[
                            {column:"type", dir:"asc"}, // Sort by issue type by default
                        ],
                        columns: [
                            {title: "Type", field: "type", width: 140, hozAlign: "center", formatter: function(cell) {
                                const type = cell.getValue();
                                let bg = "rgba(0,0,0,0.05)";
                                let color = "#333";
                                let text = "Unknown";
                                
                                // Dark mode check for pill background
                                const isDark = document.body.classList.contains('bootstrap-dark');
                                
                                switch (type) {
                                    case 0: 
                                        text = "Unsupported"; 
                                        bg = isDark ? "rgba(255,255,255,0.1)" : "#f3f2f1";
                                        color = isDark ? "#ccc" : "#555";
                                        break;
                                    case 1: 
                                        text = "Duplicate"; 
                                        bg = isDark ? "rgba(0, 120, 212, 0.2)" : "rgba(0, 120, 212, 0.1)"; 
                                        color = isDark ? "#6CB8F6" : "#0078D4"; // Fluent Blue
                                        break;
                                    case 2: 
                                        text = "Obsolete"; 
                                        bg = isDark ? "rgba(209, 52, 56, 0.2)" : "rgba(232, 17, 35, 0.1)";
                                        color = isDark ? "#FF99A4" : "#E81123"; // Fluent Red
                                        break;
                                    case 3: 
                                        text = "Unrecognised"; 
                                        bg = isDark ? "rgba(247, 99, 12, 0.2)" : "rgba(247, 99, 12, 0.1)";
                                        color = isDark ? "#FCE100" : "#9D5D00"; // Fluent Orange
                                        break;
                                    case 4: 
                                        text = "Malformed"; 
                                        bg = isDark ? "rgba(209, 52, 56, 0.2)" : "rgba(232, 17, 35, 0.1)";
                                        color = isDark ? "#FF99A4" : "#E81123"; // Fluent Red
                                        break;
                                    case 5: 
                                        text = "Missing Base"; 
                                        bg = isDark ? "rgba(209, 52, 56, 0.2)" : "rgba(232, 17, 35, 0.1)";
                                        color = isDark ? "#FF99A4" : "#E81123"; // Fluent Red
                                        break;
                                }
                                return `<div style="background-color: ${bg}; color: ${color}; font-size: 12px; font-weight: 600; padding: 4px 8px; border-radius: 12px; display: inline-block; line-height: 1;">${text}</div>`;
                            }},
                            {title: "File name",width:500, field: "key",formatter:fluentFileFormatter,cellClick:function(e, cell){
                                    //e - the click event object
                                    //cell - cell component
                                    shell.showItemInFolder(cell.getData().key)
                                }
                            },
                            {
                                title: "Issue", field: "value", formatter: function (cell) {
                                    return cell.getValue()
                                        .replaceAll("\nNew: ", "<br/><strong style='color:#0078D4; margin-top:8px; display:inline-block'>New:</strong> ")
                                        .replaceAll("\nOld: ", "<br/><strong style='color:#E81123; margin-top:4px; display:inline-block'>Old:</strong> ")
                                        .replaceAll("\nExisting: ", "<br/><strong style='color:#0078D4; margin-top:8px; display:inline-block'>Existing:</strong> ")
                                        .replaceAll("\nDuplicate: ", "<br/><strong style='color:#E81123; margin-top:4px; display:inline-block'>Duplicate:</strong> ")
                                        .replaceAll("\n", "<br/>");
                                }
                            }
                        ],
                    });
                }
            } else if (target === "#library") {
                if (state.settings.folder && !state.library){
                    return
                }
                let html = $(target + "Template").render(
                    {
                        folder: state.settings.folder,
                        library: state.library ? state.library.library_data : [] ,
                        num_skipped:state.library ? (state.library.issues ? state.library.issues.length : 0) : 0,
                        num_files:state.library ? state.library.num_files : 0,
                        keys:state.keys,
                        scanFolders:state.settings.scan_folders
                    })
                $(target).html(html);
                if (state.library && state.library.library_data.length) {
                    currTable = new Tabulator("#library-table", {
                        initialSort:[
                            {column:"name", dir:"asc"}, //sort by this first
                        ],
                        layout:"fitDataStretch",
                        headerSortTristate: true,
                        pagination: "local",
                        paginationSize: state.settings.gui_page_size,
                        data: state.library.library_data,
                        columns: [
                            {title: "Game", field: "name", headerFilter:"input", formatter:fluentTitleFormatter, width:400},
                            {title: "Title ID", field: "titleId"},
                            {title: "Region", field: "region"},
                            {title: "Location", field: "locationTag", headerFilter:"list", headerFilterParams:{valuesLookup:true}, headerFilterPlaceholder:"Filter..."},
                            {title: "Tags", field: "customTags", headerFilter:"list", headerFilterParams:{valuesLookup:true, valuesLookupField:"customTags"}, headerFilterFunc: function(headerValue, rowValue) {
                                if (!headerValue) return true;
                                if (!rowValue || !Array.isArray(rowValue)) return false;
                                return rowValue.some(tag => tag === headerValue);
                            }, formatter: function(cell) {
                                const tags = cell.getValue();
                                if (!tags || !Array.isArray(tags) || tags.length === 0) return "";
                                const maxShow = 5;
                                let html = '<div style="display:flex; flex-wrap:wrap; gap:4px; padding:2px 0;">';
                                const shown = tags.slice(0, maxShow);
                                shown.forEach(tag => {
                                    html += '<span style="background:#0078D4; color:#fff; font-size:11px; padding:2px 8px; border-radius:10px; font-weight:500;">' + tag + '</span>';
                                });
                                if (tags.length > maxShow) {
                                    html += '<span style="background:#666; color:#fff; font-size:11px; padding:2px 6px; border-radius:10px; font-weight:500;">+' + (tags.length - maxShow) + '</span>';
                                }
                                html += '</div>';
                                return html;
                            }},
                            {title: "Type", field: "type"},
                            {title: "Update", field: "update"},
                            {title: "Version", field: "version"},
                            {title: "File name", field: "path",formatter:fluentFileFormatter,cellClick:function(e, cell){
                                    //e - the click event object
                                    //cell - cell component
                                    shell.showItemInFolder(cell.getData().path)
                                }
                            }
                        ],
                    });
                    currTable.on("rowContext", function(e, row){
                        e.preventDefault();
                        const data = row.getData();
                        const titleId = data.titleId || "";
                        const name = data.name || "Unknown";
                        contextMenu.show(e.pageX, e.pageY, [
                            {label: "Add Tag to \"" + name + "\"", field: titleId, visible: null, disabled: !titleId, onClick: function(item) {
                                const tagName = prompt("Enter tag name (or select existing):\n\nExisting tags will be loaded from the tag store.");
                                if (tagName && tagName.trim()) {
                                    // First create the tag if it doesn't exist, then assign it
                                    sendMessage("saveTags", JSON.stringify({action: "create", titleId: "", tagName: tagName.trim()}), function(r) {
                                        // Now assign to the game
                                        sendMessage("saveTags", JSON.stringify({action: "add", titleId: item.field, tagName: tagName.trim()}), function(r2) {
                                            // Refresh library to show updated tags
                                            state.library = undefined;
                                            scanLocalFolder(true);
                                        });
                                    });
                                }
                            }}
                        ]);
                    });
                }
            } else if (target === "#missing") {
                if (state.settings.folder && !state.library){
                    return
                }
                if (state.library && !state.missingGames){
                    sendMessage("missingGames", "", (r => {
                        state.missingGames = JSON.parse(r)
                        loadTab("#missing")
                    }));
                    return
                }
                let html = $(target + "Template").render({folder: state.settings.folder,missingGames:state.missingGames});
                $(target).html(html);
                if (state.missingGames && state.missingGames.length) {
                    currTable = new Tabulator("#missingGames-table", {
                        layout:"fitDataStretch",
                        headerSortTristate: true,
                        initialSort:[
                            {column:"name", dir:"asc"}, //sort by this first
                        ],
                        pagination: "local",
                        paginationSize: state.settings.gui_page_size,
                        data: state.missingGames,
                        columns: [
                            {field: "name", title: "Game", headerFilter:"input", formatter:fluentTitleFormatter, width:400},
                            {title: "Title ID", field: "titleId"},
                            {title: "Region", headerFilter:"input",formatter:"textarea", field: "region"},
                            {title: "Release date", field: "release_date", sorter:"date", sorterParams:{format:"YYYY-MM-DD"}},
                        ],
                    });
                }
            }
        }

        $("body").on("click", ".folder-set", e => {
            openFolderPicker(e.target.textContent.toLowerCase().trim())
        });

        $("body").on("click", ".export-btn", e => {
            currTable.download("csv", "export.csv", {}, "all");
        });

        // Settings Form Submit
        $("body").on("submit", "#settings-form", function(e) {
            e.preventDefault();
            const formData = new FormData(this);
            
            state.settings.prod_keys = formData.get("prod_keys");
            state.settings.gui_page_size = parseInt(formData.get("gui_page_size"));
            state.settings.scan_recursively = formData.has("scan_recursively");
            state.settings.debug = formData.has("debug");
            
            state.settings.check_for_missing_updates = formData.has("check_for_missing_updates");
            state.settings.check_for_missing_dlc = formData.has("check_for_missing_dlc");
            state.settings.hide_missing_games = formData.has("hide_missing_games");
            state.settings.hide_demo_games = formData.has("hide_demo_games");
            state.settings.ignore_dlc_updates = formData.has("ignore_dlc_updates");
            
            state.settings.titles_json_url = formData.get("titles_json_url");
            state.settings.versions_json_url = formData.get("versions_json_url");
            
            const splitComma = (val) => val ? val.split(',').map(s => s.trim()).filter(s => s) : [];
            const splitNewline = (val) => val ? val.split(/\r?\n/).map(s => s.trim()).filter(s => s) : [];
            
            state.settings.ignore_file_types = splitComma(formData.get("ignore_file_types"));
            state.settings.ignore_update_title_ids = splitNewline(formData.get("ignore_update_title_ids"));
            state.settings.ignore_dlc_title_ids = splitNewline(formData.get("ignore_dlc_title_ids"));
            
            state.settings.archive_folder = formData.get("archive_folder") || "";
            
            const btn = $(this).find("button[type='submit']");
            const originalText = btn.text();
            btn.text("Saving...").prop("disabled", true);
            
            sendMessage("saveSettings", JSON.stringify(state.settings), function() {
                btn.text("Saved!").css({"background-color": "#107C10", "color": "white", "border-color": "transparent"});
                setTimeout(() => {
                    btn.text(originalText).css({"background-color": "", "color": "", "border-color": ""}).prop("disabled", false);
                }, 2000);
                
                if(state.settings.hide_missing_games){
                    document.getElementById("tab_btns").classList.add("hide_missing_games");
                } else {
                    document.getElementById("tab_btns").classList.remove("hide_missing_games");
                }

                // Save location display names
                const locationInputs = document.querySelectorAll('.location-display-name');
                locationInputs.forEach(input => {
                    const folder = input.dataset.folder;
                    const displayName = input.value.trim();
                    if (displayName) {
                        sendMessage("saveTags", JSON.stringify({action: "setLocationName", titleId: folder, tagName: displayName}), function(){});
                    }
                });
            });
        });

        // Organize Form Submit
        $("body").on("submit", "#organize-form", function(e) {
            e.preventDefault();
            const formData = new FormData(this);
            
            state.settings.organize_options.create_folder_per_game = formData.has("create_folder_per_game");
            state.settings.organize_options.rename_files = formData.has("rename_files");
            state.settings.organize_options.delete_empty_folders = formData.has("delete_empty_folders");
            state.settings.organize_options.delete_old_update_files = formData.has("delete_old_update_files");
            state.settings.organize_options.process_when_missing_base_game = formData.has("process_when_missing_base_game");
            state.settings.organize_options.switch_safe_file_names = formData.has("switch_safe_file_names");
            state.settings.organize_options.prioritize_compressed = formData.has("prioritize_compressed");
            
            state.settings.organize_options.folder_name_template = formData.get("folder_name_template");
            state.settings.organize_options.file_name_template = formData.get("file_name_template");
            state.settings.organize_options.updates_folder = formData.get("updates_folder");
            state.settings.organize_options.dlc_folder = formData.get("dlc_folder");
            
            sendMessage("saveSettings", JSON.stringify(state.settings), function() {
                if (state.settings.organize_options.create_folder_per_game === false &&
                    state.settings.organize_options.rename_files === false){
                    dialog.showMessageBox(null, {
                        type: 'info',
                        buttons: ['Ok'],
                        defaultId: 0,
                        title: 'Library organization is turned off',
                        message: 'Both rename files and create folders are disabled.',
                        detail: "You must enable at least one of these options to organize."
                    });
                    return;
                }

                const options = {
                    type: 'warning',
                    buttons: ['Yes', 'No'],
                    defaultId: 0,
                    title: 'Confirmation',
                    message: 'Are you sure you want to begin library organization?',
                    detail: 'This action will modify your local library files based on the settings you just chose.',
                };

                dialog.showMessageBox(null, options).then( (r) => {
                    if (r.response === 0) {
                        $('.tabgroup > div').hide();
                        $(".progress-container").show();
                        $(".progress-type").text("Organizing local library...");

                        sendMessage("organize", "", (r => {
                            $(".progress-container").hide();
                            state.library = undefined;
                            state.updates = undefined;
                            state.dlc = undefined;
                            loadTab("#library");
                            scanLocalFolder(true);
                            dialog.showMessageBox(null, {
                                type: 'info',
                                buttons: ['Ok'],
                                defaultId: 0,
                                title: 'Success',
                                message: 'Operation completed successfully'
                            });
                        }));
                    }
                });
            });
        });

        // Library & Issues Tab Organize Buttons
        $("body").on("click", ".library-organize-action", e => {
            e.preventDefault();
            if (state.settings.organize_options.create_folder_per_game === false &&
                state.settings.organize_options.rename_files === false){
                dialog.showMessageBox(null, {
                    type: 'info',
                    buttons: ['Ok'],
                    defaultId: 0,
                    title: 'Library organization is turned off',
                    message: 'Both rename files and create folders are disabled.',
                    detail: "You must enable at least one of these options in the Organize tab to proceed."
                });
                return;
            }

            const options = {
                type: 'warning',
                buttons: ['Yes', 'No'],
                defaultId: 0,
                title: 'Confirmation',
                message: 'Are you sure you want to begin library organization?',
                detail: 'This action will modify your local library files based on your current settings.',
            };

            dialog.showMessageBox(null, options).then( (r) => {
                if (r.response === 0) {
                    $('.tabgroup > div').hide();
                    $(".progress-container").show();
                    $(".progress-type").text("Organizing local library...");

                    sendMessage("organize", "", (r => {
                        $(".progress-container").hide();
                        state.library = undefined;
                        state.updates = undefined;
                        state.dlc = undefined;
                        loadTab("#library");
                        scanLocalFolder(true);
                        dialog.showMessageBox(null, {
                            type: 'info',
                            buttons: ['Ok'],
                            defaultId: 0,
                            title: 'Success',
                            message: 'Operation completed successfully'
                        });
                    }));
                }
            });
        });

        // Dark Mode Toggle
        $("body").on("click", "#toggle-dark-mode", e => {
            e.preventDefault();
            state.settings.dark_mode = !state.settings.dark_mode;
            
            setDarkMode(state.settings.dark_mode);
            
            // Save the toggle preference without scanning
            sendMessage("saveSettings", JSON.stringify(state.settings), function(){});
        });

        // Rescan Library Toggle
        $("body").on("click", "#btn-rescan", e => {
            e.preventDefault();
            state.library = undefined;
            state.updates = undefined;
            state.dlc = undefined;
            scanLocalFolder(true);
        });

        // Hard Rescan Toggle
        $("body").on("click", "#btn-hard-rescan", e => {
            e.preventDefault();
            const options = {
                type: 'warning',
                buttons: ['Yes', 'No'],
                defaultId: 0,
                title: 'Confirmation',
                message: 'Are you sure you want to perform a Hard Rescan?',
                detail: 'This will completely clear the local database cache and do a deep scan of all your files again. It will take longer than a normal rescan.',
            };
            dialog.showMessageBox(null, options).then( (r) => {
                if (r.response === 0) {
                    sendMessage("hardRescan", "", function(){});
                }
            });
        });



        $('#tab_btns a').click(function (e) {
            e.preventDefault();
            let target = $(e.currentTarget).attr('href');
            if (target === "#") return; // Ignore icon buttons in navbar
            loadTab(target);
        });

        function hideCurrentTab() {
            $("#tab_btns a").removeClass("active");
            let tabgroup = $("#tab_btns").data('tabgroup');
            $("#" + tabgroup).children('div').hide();
        }

        function partial(func /*, 0..n args */) {
            var args = Array.prototype.slice.call(arguments, 1);
            return function() {
                var allArguments = args.concat(Array.prototype.slice.call(arguments));
                return func.apply(this, allArguments);
            };
        }

        // Track Window Dimensions
        let resizeTimer;
        window.addEventListener('resize', () => {
            clearTimeout(resizeTimer);
            resizeTimer = setTimeout(() => {
                try {
                    const win = require('electron').remote.getCurrentWindow();
                    const bounds = win.getBounds();
                    const isMax = win.isMaximized();
                    state.settings.window_maximized = isMax;
                    if (!isMax) {
                        state.settings.window_width = bounds.width;
                        state.settings.window_height = bounds.height;
                    }
                    sendMessage("saveSettings", JSON.stringify(state.settings), function(){});
                } catch(e) {}
            }, 1000); // Save bounds 1 second after user finishes resizing
        });

    });

});