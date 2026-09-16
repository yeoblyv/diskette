package locales

import Graphite "github.com/yeoblyv/graphite"

// English is the reference catalog every other language is translated
// from — it also doubles as the fallback Application.T already applies
// on a missing key in any other locale, so every key defined in keys.go
// must have an entry here.
var English = Graphite.Catalog{
	KeyErrorTitle: " Error ",
	KeyCancel:     "Cancel",
	KeyName:       "Name:",

	KeyMenuFile:    "File",
	KeyMenuMark:    "Mark",
	KeyMenuView:    "View",
	KeyMenuTab:     "Tab",
	KeyMenuNetwork: "Network",
	KeyMenuHelp:    "Help",

	KeyMenuFileInfo:   "File Info     F1",
	KeyMenuFileRename: "Rename        F2",
	KeyMenuFileFind:   "Find          F3",
	KeyMenuFileGrep:   "Grep          F4",
	KeyMenuFileCopy:   "Copy          F5",
	KeyMenuFileMove:   "Move          F6",
	KeyMenuFileNew:    "New File",
	KeyMenuFileMkdir:  "New Folder    F7",
	KeyMenuFileDelete: "Delete        F8",
	KeyMenuFileQuit:   "Quit         F10",

	KeyMenuMarkToggle: "Tag/Untag    Ins",
	KeyMenuMarkAll:    "Select All",
	KeyMenuMarkNone:   "Deselect All",
	KeyMenuMarkInvert: "Invert Selection",

	KeyMenuViewSortName: "Sort by Name",
	KeyMenuViewSortExt:  "Sort by Extension",
	KeyMenuViewSortSize: "Sort by Size",
	KeyMenuViewSortDate: "Sort by Date",
	KeyMenuViewRefresh:  "Refresh",

	KeyMenuTabAdd:         "Add",
	KeyMenuTabAddFileList: "New file list",
	KeyMenuTabAddTerminal: "New terminal",
	KeyMenuTabPinUnpin:    "Pin/Unpin Tab",
	KeyMenuTabClose:       "Close Tab",
	KeyMenuTabManage:      "Manage Tabs...",

	KeyMenuNetworkCreate:     "Create Connection...",
	KeyMenuNetworkReconnect:  "Reconnect",
	KeyMenuNetworkDisconnect: "Disconnect",

	KeyMenuHelpLanguage: "Language",
	KeyMenuHelpAbout:    "About",

	KeyFKeyInfo:   "Info",
	KeyFKeyRename: "Rename",
	KeyFKeyFind:   "Find",
	KeyFKeyGrep:   "Grep",
	KeyFKeyCopy:   "Copy %s",
	KeyFKeyMove:   "Move %s",
	KeyFKeyMkdir:  "MkDir",
	KeyFKeyDelete: "Delete",
	KeyFKeyMenu:   "Menu",
	KeyFKeyQuit:   "Quit",

	KeyDirectionLeft:  "Left",
	KeyDirectionRight: "Right",

	KeyGutterCopy:  "Copy",
	KeyGutterMove:  "Move",
	KeyGutterZip:   "Zip",
	KeyGutterUnzip: "Unzip",

	KeyNavRoot: "Root",

	KeyStatusNoTagged:    "No files tagged",
	KeyStatusTagged:      "Tagged: %d item(s), %s",
	KeyStatusServer:      "Server: %s",
	KeyStatusServerLocal: "Local",
	KeyStatusDiskNA:      "Disk: n/a",
	KeyStatusDiskUsage:   "Disk: %s free of %s (%.0f%% used)",
	KeyStatusSearch:      "Search: %s",

	KeyFilePaneColName: "Name",
	KeyFilePaneColExt:  "Ext",
	KeyFilePaneColSize: "Size",
	KeyFilePaneColDate: "Date",
	KeyFilePaneColAttr: "Attr",

	KeyFilePaneStatusPlain:  "%d file(s), %d dir(s)",
	KeyFilePaneStatusTagged: "%d file(s), %d dir(s) — %d tagged (%s)",

	KeyTabTerminal: "Terminal",
	KeyTabRoot:     "Root",

	KeyAboutVersion:  "Diskette v.%s",
	KeyAboutTagline1: "A cross-platform dual-pane file manager,",
	KeyAboutTagline2: "with SFTP and FTP/FTPS remote connections.",
	KeyAboutTagline3: "Provides archiving features and build with",
	KeyAboutTagline4: "lightweight Graphite TUI framework.",
	KeyAboutBeta:     "Beta release.",
	KeyClose:         "Close",

	KeyFileInfoTitle:    " File Info ",
	KeyFileInfoMulti:    "%d items selected\n\nTotal size: %s",
	KeyFileInfoName:     "Name:        %s",
	KeyFileInfoType:     "Type:        %s",
	KeyFileInfoSize:     "Size:        %s",
	KeyFileInfoPerms:    "Permissions: %s",
	KeyFileInfoModified: "Modified:    %s",
	KeyFileInfoAccessed: "Accessed:    %s",
	KeyFileInfoCreated:  "Created:     %s",
	KeyFileInfoNotAvail: "not available on this filesystem",
	KeyFileInfoKindFile: "File",
	KeyFileInfoKindDir:  "Directory",

	KeyFindTitle:        " Find Files ",
	KeyFindResultsTitle: " Find Files ",
	KeySearchIn:         "Search in: ",
	KeyNameMask:         "Name mask: ",
	KeySearchSubfolders: "Search subfolders",
	KeyCaseSensitive:    "Case sensitive",
	KeySearch:           "Search",
	KeySearching:        "Searching…",
	KeySearchFound:      "%d found",
	KeySearchingFound:   "Searching… %d found",

	KeyGrepTitle:        " Grep ",
	KeyGrepResultsTitle: " Grep ",
	KeyGrepPattern:      "Pattern: ",
	KeyGrepRegex:        "Regular expression",

	KeyChooseRootTitle: " Choose root ",

	KeyCopyTitle: " Copy ",
	KeyMoveTitle: " Move ",

	KeyZipTitle:         " Zip ",
	KeyUnzipTitle:       " Unzip ",
	KeyArchiveName:      "Archive name:",
	KeyArchiveOverwrite: "%s already exists. Overwrite?",
	KeyNotAnArchive:     "Not a recognized archive: %s",

	KeyConflictTitle:     " Conflict ",
	KeyConflictExists:    "Already exists:",
	KeyConflictApplyAll:  "Apply to all",
	KeyConflictOverwrite: "Overwrite",
	KeyConflictSkip:      "Skip",
	KeyConflictRename:    "Rename",

	KeyManageTabsTitle:    " Manage Tabs ",
	KeyManageTabsLeft:     "Left",
	KeyManageTabsRight:    "Right",
	KeyManageTabsAddFiles: "+ Files",
	KeyManageTabsAddTerm:  "+ Term",
	KeyManageTabsCloseTab: "Close",
	KeyManageTabsDone:     "Done",

	KeyGotoFolderTitle: " Go to folder ",
	KeyPath:            "Path:",

	KeyRenameTitle: " Rename ",
	KeyNewName:     "New name:",

	KeyNewFileTitle:   " New file ",
	KeyNewFolderTitle: " New folder ",
	KeyAlreadyExists:  "A file or folder named \"%s\" already exists.",

	KeyQuitTitle:     " Quit ",
	KeyQuitMessage:   "Quit Diskette?",
	KeyDeleteTitle:   " Delete ",
	KeyDeleteMessage: "Delete %d item(s)?",

	KeyErrRemoteEditUnsupported: "Opening a remote file isn't supported yet.",

	KeyConnectTitle:      " Connect to server ",
	KeyConnectProtocol:   "Protocol:",
	KeyConnectAuthMethod: "Auth method:",
	KeyConnectSecurity:   "Security:",
	KeyConnectHost:       "Host:      ",
	KeyConnectPort:       "Port:      ",
	KeyConnectUsername:   "Username:  ",
	KeyConnectKeyFile:    "Key file:  ",
	KeyConnectPassphrase: "Passphrase:",
	KeyConnectPassword:   "Password:  ",
	KeyConnectButton:     "Connect",

	KeyAuthPassword:   "Password",
	KeyAuthPrivateKey: "Private key",
	KeyAuthAgent:      "SSH agent",

	KeyTLSNone:     "None",
	KeyTLSExplicit: "FTPS (explicit)",
	KeyTLSImplicit: "FTPS (implicit)",

	KeyErrPortNumber:   "Port must be a number.",
	KeyErrHostEmpty:    "Host cannot be empty.",
	KeyErrNotConnected: "The active tab isn't connected to a server.",

	KeyHostKeyTitle:   " Unknown Host ",
	KeyHostKeyMessage: "The authenticity of host '%s' can't be established.\n%s key fingerprint:\n%s\n\nTrust this key and continue connecting?",
	KeyHostKeyTrust:   "Trust",

	KeyCLIHelpIntro:    "diskette: this shell is running inside a diskette terminal tab, so \"diskette\" here talks back to the open instance instead of starting a new one.",
	KeyCLIHelpCommands: "Available commands:",
	KeyCLIHelpView:     "  diskette view [path]      navigate the other pane to path (default: here)",
	KeyCLIHelpTag:      "  diskette tag <name...>    tag entries by name in the other pane",
	KeyCLIHelpUntag:    "  diskette untag <name...>  untag entries by name in the other pane",
	KeyCLIHelpSelect:   "  diskette select <name>    move the cursor to one entry in the other pane",
	KeyCLIHelpSync:     "  diskette sync on|off      keep the other pane's path synced to $PWD",
}
