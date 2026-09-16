package locales

import Graphite "github.com/yeoblyv/graphite"

// Dutch. Any key not listed here (e.g. KeyMenuFileGrep, whose value is
// just "Grep" — a tool name kept as-is in every language, the same way
// "SFTP"/"FTP" aren't translated either) falls back to English, per
// Application.T's resolution order.
var Dutch = Graphite.Catalog{
	KeyErrorTitle: " Fout ",
	KeyCancel:     "Annuleren",
	KeyName:       "Naam:",

	KeyMenuFile:    "Bestand",
	KeyMenuMark:    "Markeren",
	KeyMenuView:    "Beeld",
	KeyMenuTab:     "Tabblad",
	KeyMenuNetwork: "Netwerk",
	KeyMenuHelp:    "Help",

	KeyMenuFileInfo:   "Bestandsinfo         F1",
	KeyMenuFileRename: "Hernoemen            F2",
	KeyMenuFileFind:   "Zoeken               F3",
	KeyMenuFileCopy:   "Kopiëren             F5",
	KeyMenuFileMove:   "Verplaatsen          F6",
	KeyMenuFileNew:    "Nieuw bestand",
	KeyMenuFileMkdir:  "Nieuwe map           F7",
	KeyMenuFileDelete: "Verwijderen          F8",
	KeyMenuFileQuit:   "Afsluiten           F10",

	KeyMenuMarkToggle: "Markeren/demarkeren  Ins",
	KeyMenuMarkAll:    "Alles selecteren",
	KeyMenuMarkNone:   "Selectie opheffen",
	KeyMenuMarkInvert: "Selectie omkeren",

	KeyMenuViewSortName: "Sorteren op naam",
	KeyMenuViewSortExt:  "Sorteren op extensie",
	KeyMenuViewSortSize: "Sorteren op grootte",
	KeyMenuViewSortDate: "Sorteren op datum",
	KeyMenuViewRefresh:  "Vernieuwen",

	KeyMenuTabAdd:         "Toevoegen",
	KeyMenuTabAddFileList: "Nieuwe bestandslijst",
	KeyMenuTabAddTerminal: "Nieuwe terminal",
	KeyMenuTabPinUnpin:    "Tabblad vastzetten/losmaken",
	KeyMenuTabClose:       "Tabblad sluiten",
	KeyMenuTabManage:      "Tabbladen beheren...",

	KeyMenuNetworkCreate:     "Verbinding maken...",
	KeyMenuNetworkReconnect:  "Opnieuw verbinden",
	KeyMenuNetworkDisconnect: "Verbinding verbreken",

	KeyMenuHelpLanguage: "Taal",
	KeyMenuHelpAbout:    "Over",

	KeyFKeyInfo:   "Info",
	KeyFKeyRename: "Hernoem",
	KeyFKeyFind:   "Zoeken",
	KeyFKeyCopy:   "Kopieer %s",
	KeyFKeyMove:   "Verplaats %s",
	KeyFKeyMkdir:  "NwMap",
	KeyFKeyDelete: "Verwijder",
	KeyFKeyMenu:   "Menu",
	KeyFKeyQuit:   "Afsluiten",

	KeyDirectionLeft:  "links",
	KeyDirectionRight: "rechts",

	KeyGutterCopy: "Kopiëren",
	KeyGutterMove: "Verplaatsen",

	KeyNavRoot: "Root",

	KeyStatusNoTagged:    "Geen bestanden gemarkeerd",
	KeyStatusTagged:      "Gemarkeerd: %d item(s), %s",
	KeyStatusServer:      "Server: %s",
	KeyStatusServerLocal: "Lokaal",
	KeyStatusDiskNA:      "Schijf: n.v.t.",
	KeyStatusDiskUsage:   "Schijf: %s vrij van %s (%.0f%% gebruikt)",
	KeyStatusSearch:      "Zoeken: %s",

	KeyFilePaneColName: "Naam",
	KeyFilePaneColExt:  "Ext",
	KeyFilePaneColSize: "Grootte",
	KeyFilePaneColDate: "Datum",
	KeyFilePaneColAttr: "Attr",

	KeyFilePaneStatusPlain:  "%d bestand(en), %d map(pen)",
	KeyFilePaneStatusTagged: "%d bestand(en), %d map(pen) — %d gemarkeerd (%s)",

	KeyTabTerminal: "Terminal",
	KeyTabRoot:     "Root",

	KeyAboutTagline1: "Een cross-platform bestandsbeheerder met twee panelen,",
	KeyAboutTagline2: "met SFTP- en FTP/FTPS-externe verbindingen.",
	KeyAboutTagline3: "Biedt archiveringsfuncties en is gebouwd op",
	KeyAboutTagline4: "het lichtgewicht Graphite TUI-framework.",
	KeyAboutBeta:     "Bètaversie.",
	KeyClose:         "Sluiten",

	KeyFileInfoTitle:    " Bestandsinfo ",
	KeyFileInfoMulti:    "%d items geselecteerd\n\nTotale grootte: %s",
	KeyFileInfoName:     "Naam:              %s",
	KeyFileInfoType:     "Type:              %s",
	KeyFileInfoSize:     "Grootte:           %s",
	KeyFileInfoPerms:    "Machtigingen:      %s",
	KeyFileInfoModified: "Gewijzigd:         %s",
	KeyFileInfoAccessed: "Geopend:           %s",
	KeyFileInfoCreated:  "Gemaakt:           %s",
	KeyFileInfoNotAvail: "niet beschikbaar op dit bestandssysteem",
	KeyFileInfoKindFile: "Bestand",
	KeyFileInfoKindDir:  "Map",

	KeyFindTitle:        " Bestanden zoeken ",
	KeyFindResultsTitle: " Bestanden zoeken ",
	KeySearchIn:         "Zoeken in: ",
	KeyNameMask:         "Naammasker: ",
	KeySearchSubfolders: "Submappen doorzoeken",
	KeyCaseSensitive:    "Hoofdlettergevoelig",
	KeySearch:           "Zoeken",
	KeySearching:        "Zoeken…",
	KeySearchFound:      "%d gevonden",
	KeySearchingFound:   "Zoeken… %d gevonden",

	KeyGrepPattern: "Patroon: ",
	KeyGrepRegex:   "Reguliere expressie",

	KeyChooseRootTitle: " Kies root ",

	KeyCopyTitle: " Kopiëren ",
	KeyMoveTitle: " Verplaatsen ",

	KeyArchiveName:      "Archiefnaam:",
	KeyArchiveOverwrite: "%s bestaat al. Overschrijven?",
	KeyNotAnArchive:     "Geen herkend archief: %s",

	KeyConflictTitle:     " Conflict ",
	KeyConflictExists:    "Bestaat al:",
	KeyConflictApplyAll:  "Toepassen op alles",
	KeyConflictOverwrite: "Overschrijven",
	KeyConflictSkip:      "Overslaan",
	KeyConflictRename:    "Hernoemen",

	KeyManageTabsTitle:    " Tabbladen beheren ",
	KeyManageTabsLeft:     "Links",
	KeyManageTabsRight:    "Rechts",
	KeyManageTabsAddFiles: "+ Best.",
	KeyManageTabsAddTerm:  "+ Term",
	KeyManageTabsCloseTab: "Sluiten",
	KeyManageTabsDone:     "Klaar",

	KeyGotoFolderTitle: " Naar map gaan ",
	KeyPath:            "Pad:",

	KeyRenameTitle: " Hernoemen ",
	KeyNewName:     "Nieuwe naam:",

	KeyNewFileTitle:   " Nieuw bestand ",
	KeyNewFolderTitle: " Nieuwe map ",
	KeyAlreadyExists:  "Een bestand of map met de naam \"%s\" bestaat al.",

	KeyQuitTitle:     " Afsluiten ",
	KeyQuitMessage:   "Diskette afsluiten?",
	KeyDeleteTitle:   " Verwijderen ",
	KeyDeleteMessage: "%d item(s) verwijderen?",

	KeyErrRemoteEditUnsupported: "Een extern bestand openen wordt nog niet ondersteund.",
	KeyErrDestinationInsideSrc:  "Kan niet worden voltooid: de bestemming is dezelfde map, of bevindt zich binnen de map die wordt gekopieerd of verplaatst.",

	KeyConnectTitle:      " Verbinden met server ",
	KeyConnectProtocol:   "Protocol:",
	KeyConnectAuthMethod: "Authenticatiemethode:",
	KeyConnectSecurity:   "Beveiliging:",
	KeyConnectHost:       "Host:            ",
	KeyConnectPort:       "Poort:           ",
	KeyConnectUsername:   "Gebruikersnaam:  ",
	KeyConnectKeyFile:    "Sleutelbestand:  ",
	KeyConnectPassphrase: "Wachtwoordzin:   ",
	KeyConnectPassword:   "Wachtwoord:      ",
	KeyConnectButton:     "Verbinden",

	KeyAuthPassword:   "Wachtwoord",
	KeyAuthPrivateKey: "Privésleutel",
	KeyAuthAgent:      "SSH-agent",

	KeyTLSNone:     "Geen",
	KeyTLSExplicit: "FTPS (expliciet)",
	KeyTLSImplicit: "FTPS (impliciet)",

	KeyErrPortNumber:   "Poort moet een getal zijn.",
	KeyErrHostEmpty:    "Host mag niet leeg zijn.",
	KeyErrNotConnected: "Het actieve tabblad is niet verbonden met een server.",

	KeyHostKeyTitle:   " Onbekende host ",
	KeyHostKeyMessage: "De authenticiteit van host '%s' kan niet worden vastgesteld.\n%s-sleutel-vingerafdruk:\n%s\n\nDeze sleutel vertrouwen en verbinding maken?",
	KeyHostKeyTrust:   "Vertrouwen",

	KeyCLIHelpIntro:    "diskette: deze shell draait in een diskette-terminaltabblad, dus \"diskette\" praat hier terug naar de openstaande instantie in plaats van een nieuwe te starten.",
	KeyCLIHelpCommands: "Beschikbare commando's:",
	KeyCLIHelpView:     "  diskette view [pad]        navigeer het andere paneel naar pad (standaard: hier)",
	KeyCLIHelpTag:      "  diskette tag <naam...>     markeer items op naam in het andere paneel",
	KeyCLIHelpUntag:    "  diskette untag <naam...>   demarkeer items op naam in het andere paneel",
	KeyCLIHelpSelect:   "  diskette select <naam>     verplaats de cursor naar één item in het andere paneel",
	KeyCLIHelpSync:     "  diskette sync on|off       houd het pad van het andere paneel gesynchroniseerd met $PWD",
}
