// Package locales holds diskette's own translation catalogs — English,
// Ukrainian, Russian, and Dutch — for every string the program itself
// draws (as opposed to graphite's built-in Yes/No/OK/Cancel/etc., which
// graphite already translates on its own via its own built-in Catalog).
// Register all four with Application.SetTranslations at startup; see
// cmd/diskette/main.go's setupLocales.
//
// One file per language (en.go/uk.go/ru.go/nl.go), following the layout
// graphite's own docs/i18n.md recommends, with every key defined here as
// an exported constant so a typo in a Catalog literal is a compile
// error rather than a silently-missing translation. Keys are namespaced
// "diskette.<area>.<element>" so they can never collide with graphite's
// own "graphite.*" keys.
package locales

const (
	// --- Generic / shared ---

	KeyErrorTitle = "diskette.error_title"
	KeyCancel     = "diskette.cancel" // diskette's own dialogs use this instead of Graphite.KeyCancel so a caller-supplied title/button stays consistent with the rest of diskette's own UI text
	KeyName       = "diskette.name"

	// --- Menu strip: category labels ---

	KeyMenuFile    = "diskette.menu.file"
	KeyMenuMark    = "diskette.menu.mark"
	KeyMenuView    = "diskette.menu.view"
	KeyMenuTab     = "diskette.menu.tab"
	KeyMenuNetwork = "diskette.menu.network"
	KeyMenuHelp    = "diskette.menu.help"

	// --- Menu strip: File category items (each includes its own F-key
	// padding, since a translation's length changes where the column
	// needs to fall — see en.go's comment for the convention) ---

	KeyMenuFileInfo   = "diskette.menu.file.info"
	KeyMenuFileRename = "diskette.menu.file.rename"
	KeyMenuFileFind   = "diskette.menu.file.find"
	KeyMenuFileGrep   = "diskette.menu.file.grep"
	KeyMenuFileCopy   = "diskette.menu.file.copy"
	KeyMenuFileMove   = "diskette.menu.file.move"
	KeyMenuFileNew    = "diskette.menu.file.new"
	KeyMenuFileMkdir  = "diskette.menu.file.mkdir"
	KeyMenuFileDelete = "diskette.menu.file.delete"
	KeyMenuFileQuit   = "diskette.menu.file.quit"

	// --- Menu strip: Mark category ---

	KeyMenuMarkToggle = "diskette.menu.mark.toggle"
	KeyMenuMarkAll    = "diskette.menu.mark.all"
	KeyMenuMarkNone   = "diskette.menu.mark.none"
	KeyMenuMarkInvert = "diskette.menu.mark.invert"

	// --- Menu strip: View category ---

	KeyMenuViewSortName = "diskette.menu.view.sortname"
	KeyMenuViewSortExt  = "diskette.menu.view.sortext"
	KeyMenuViewSortSize = "diskette.menu.view.sortsize"
	KeyMenuViewSortDate = "diskette.menu.view.sortdate"
	KeyMenuViewRefresh  = "diskette.menu.view.refresh"

	// --- Menu strip: Tab category ---

	KeyMenuTabAdd         = "diskette.menu.tab.add"
	KeyMenuTabAddFileList = "diskette.menu.tab.addfilelist"
	KeyMenuTabAddTerminal = "diskette.menu.tab.addterminal"
	KeyMenuTabPinUnpin    = "diskette.menu.tab.pinunpin"
	KeyMenuTabClose       = "diskette.menu.tab.close"
	KeyMenuTabManage      = "diskette.menu.tab.manage"

	// --- Menu strip: Network category ---

	KeyMenuNetworkCreate     = "diskette.menu.network.create"
	KeyMenuNetworkReconnect  = "diskette.menu.network.reconnect"
	KeyMenuNetworkDisconnect = "diskette.menu.network.disconnect"

	// --- Menu strip: Help category ---

	KeyMenuHelpLanguage = "diskette.menu.help.language"
	KeyMenuHelpAbout    = "diskette.menu.help.about"

	// --- F-key bar (Text next to each F-key chip) ---

	KeyFKeyInfo   = "diskette.fkey.info"
	KeyFKeyRename = "diskette.fkey.rename"
	KeyFKeyFind   = "diskette.fkey.find"
	KeyFKeyGrep   = "diskette.fkey.grep"
	KeyFKeyCopy   = "diskette.fkey.copy" // %s: direction (Left/Right)
	KeyFKeyMove   = "diskette.fkey.move" // %s: direction (Left/Right)
	KeyFKeyMkdir  = "diskette.fkey.mkdir"
	KeyFKeyDelete = "diskette.fkey.delete"
	KeyFKeyMenu   = "diskette.fkey.menu"
	KeyFKeyQuit   = "diskette.fkey.quit"

	KeyDirectionLeft  = "diskette.direction.left"
	KeyDirectionRight = "diskette.direction.right"

	// --- Gutter buttons between panes (Copy/Move/Zip/Unzip) ---

	KeyGutterCopy  = "diskette.gutter.copy"
	KeyGutterMove  = "diskette.gutter.move"
	KeyGutterZip   = "diskette.gutter.zip"
	KeyGutterUnzip = "diskette.gutter.unzip"

	// --- Nav row ---

	KeyNavRoot = "diskette.nav.root"

	// --- Status bar / disk bar ---

	KeyStatusNoTagged    = "diskette.status.notagged"
	KeyStatusTagged      = "diskette.status.tagged" // %d item(s), %s size
	KeyStatusServer      = "diskette.status.server" // %s: label
	KeyStatusServerLocal = "diskette.status.server_local"
	KeyStatusDiskNA      = "diskette.status.disk_na"
	KeyStatusDiskUsage   = "diskette.status.disk_usage" // %s free, %s total, %.0f%% used
	KeyStatusSearch      = "diskette.status.search"     // %s: quick-search query

	// --- FilePane (column headers + row-count/tagged status line) ---

	KeyFilePaneColName = "diskette.filepane.col.name"
	KeyFilePaneColExt  = "diskette.filepane.col.ext"
	KeyFilePaneColSize = "diskette.filepane.col.size"
	KeyFilePaneColDate = "diskette.filepane.col.date"
	KeyFilePaneColAttr = "diskette.filepane.col.attr"

	KeyFilePaneStatusPlain  = "diskette.filepane.status.plain"  // %d file(s), %d dir(s)
	KeyFilePaneStatusTagged = "diskette.filepane.status.tagged" // %d file(s), %d dir(s), %d tagged, %s

	// --- Tab names ---

	KeyTabTerminal = "diskette.tab.terminal"
	KeyTabRoot     = "diskette.tab.root"

	// --- About dialog ---

	KeyAboutVersion  = "diskette.about.version" // %s: version number
	KeyAboutTagline1 = "diskette.about.tagline1"
	KeyAboutTagline2 = "diskette.about.tagline2"
	KeyAboutTagline3 = "diskette.about.tagline3"
	KeyAboutTagline4 = "diskette.about.tagline4"
	KeyAboutBeta     = "diskette.about.beta"
	KeyClose         = "diskette.close"

	// --- File Info dialog (F1) ---

	KeyFileInfoTitle    = "diskette.fileinfo.title"
	KeyFileInfoMulti    = "diskette.fileinfo.multi" // %d items selected, %s total size
	KeyFileInfoName     = "diskette.fileinfo.name"
	KeyFileInfoType     = "diskette.fileinfo.type"
	KeyFileInfoSize     = "diskette.fileinfo.size"
	KeyFileInfoPerms    = "diskette.fileinfo.perms"
	KeyFileInfoModified = "diskette.fileinfo.modified"
	KeyFileInfoAccessed = "diskette.fileinfo.accessed"
	KeyFileInfoCreated  = "diskette.fileinfo.created"
	KeyFileInfoNotAvail = "diskette.fileinfo.notavail"
	KeyFileInfoKindFile = "diskette.fileinfo.kind.file"
	KeyFileInfoKindDir  = "diskette.fileinfo.kind.dir"

	// --- Find Files dialog (F3) ---

	KeyFindTitle        = "diskette.find.title"
	KeyFindResultsTitle = "diskette.find.results_title"
	KeySearchIn         = "diskette.search.in"
	KeyNameMask         = "diskette.search.namemask"
	KeySearchSubfolders = "diskette.search.subfolders"
	KeyCaseSensitive    = "diskette.search.casesensitive"
	KeySearch           = "diskette.search.button"
	KeySearching        = "diskette.search.searching"
	KeySearchFound      = "diskette.search.found"           // %d found
	KeySearchingFound   = "diskette.search.searching_found" // searching, %d found so far

	// --- Grep dialog (F4) ---

	KeyGrepTitle        = "diskette.grep.title"
	KeyGrepResultsTitle = "diskette.grep.results_title"
	KeyGrepPattern      = "diskette.grep.pattern"
	KeyGrepRegex        = "diskette.grep.regex"

	// --- Choose root dialog ---

	KeyChooseRootTitle = "diskette.chooseroot.title"

	// --- Copy/Move progress dialog ---

	KeyCopyTitle = "diskette.copy.title"
	KeyMoveTitle = "diskette.move.title"

	// --- Zip/Unzip ---

	KeyZipTitle         = "diskette.zip.title"
	KeyUnzipTitle       = "diskette.unzip.title"
	KeyArchiveName      = "diskette.zip.archivename"
	KeyArchiveOverwrite = "diskette.zip.overwrite"    // %s already exists. Overwrite?
	KeyNotAnArchive     = "diskette.unzip.notarchive" // %s

	// --- Conflict dialog ---

	KeyConflictTitle     = "diskette.conflict.title"
	KeyConflictExists    = "diskette.conflict.exists"
	KeyConflictApplyAll  = "diskette.conflict.applyall"
	KeyConflictOverwrite = "diskette.conflict.overwrite"
	KeyConflictSkip      = "diskette.conflict.skip"
	KeyConflictRename    = "diskette.conflict.rename"

	// --- Manage Tabs dialog ---

	KeyManageTabsTitle    = "diskette.managetabs.title"
	KeyManageTabsLeft     = "diskette.managetabs.left"
	KeyManageTabsRight    = "diskette.managetabs.right"
	KeyManageTabsAddFiles = "diskette.managetabs.addfiles"
	KeyManageTabsAddTerm  = "diskette.managetabs.addterm"
	KeyManageTabsCloseTab = "diskette.managetabs.closetab"
	KeyManageTabsDone     = "diskette.managetabs.done"

	// --- Go to folder ---

	KeyGotoFolderTitle = "diskette.gotofolder.title"
	KeyPath            = "diskette.gotofolder.path"

	// --- Rename dialog ---

	KeyRenameTitle = "diskette.rename.title"
	KeyNewName     = "diskette.rename.newname"

	// --- New file / New folder ---

	KeyNewFileTitle   = "diskette.newfile.title"
	KeyNewFolderTitle = "diskette.newfolder.title"
	KeyAlreadyExists  = "diskette.newfile.alreadyexists" // %s

	// --- Quit / Delete confirmation ---

	KeyQuitTitle     = "diskette.quit.title"
	KeyQuitMessage   = "diskette.quit.message"
	KeyDeleteTitle   = "diskette.delete.title"
	KeyDeleteMessage = "diskette.delete.message" // %d item(s)?

	// --- Misc errors ---

	KeyErrRemoteEditUnsupported = "diskette.error.remote_open_unsupported"

	// --- Connect dialog ---

	KeyConnectTitle      = "diskette.connect.title"
	KeyConnectProtocol   = "diskette.connect.protocol"
	KeyConnectAuthMethod = "diskette.connect.authmethod"
	KeyConnectSecurity   = "diskette.connect.security"
	KeyConnectHost       = "diskette.connect.host"
	KeyConnectPort       = "diskette.connect.port"
	KeyConnectUsername   = "diskette.connect.username"
	KeyConnectKeyFile    = "diskette.connect.keyfile"
	KeyConnectPassphrase = "diskette.connect.passphrase"
	KeyConnectPassword   = "diskette.connect.password"
	KeyConnectButton     = "diskette.connect.button"

	KeyAuthPassword   = "diskette.connect.auth.password"
	KeyAuthPrivateKey = "diskette.connect.auth.privatekey"
	KeyAuthAgent      = "diskette.connect.auth.agent"

	KeyTLSNone     = "diskette.connect.tls.none"
	KeyTLSExplicit = "diskette.connect.tls.explicit"
	KeyTLSImplicit = "diskette.connect.tls.implicit"

	KeyErrPortNumber   = "diskette.connect.err.portnumber"
	KeyErrHostEmpty    = "diskette.connect.err.hostempty"
	KeyErrNotConnected = "diskette.connect.err.notconnected"

	KeyHostKeyTitle   = "diskette.connect.hostkey.title"
	KeyHostKeyMessage = "diskette.connect.hostkey.message" // hostname, key type, fingerprint
	KeyHostKeyTrust   = "diskette.connect.hostkey.trust"

	// --- cli.go help text (printed inside an embedded terminal tab; see
	// cmd/diskette/cli.go — this process has no *Graphite.Application, so
	// these are resolved by a small standalone lookup against the same
	// Catalog values, keyed by $DISKETTE_LOCALE, not by Application.T) ---

	KeyCLIHelpIntro    = "diskette.cli.help.intro"
	KeyCLIHelpCommands = "diskette.cli.help.commands"
	KeyCLIHelpView     = "diskette.cli.help.view"
	KeyCLIHelpTag      = "diskette.cli.help.tag"
	KeyCLIHelpUntag    = "diskette.cli.help.untag"
	KeyCLIHelpSelect   = "diskette.cli.help.select"
	KeyCLIHelpSync     = "diskette.cli.help.sync"
)
