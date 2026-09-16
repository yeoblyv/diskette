package locales

import Graphite "github.com/yeoblyv/graphite"

// Ukrainian. Any key not listed here (e.g. KeyMenuFileGrep, whose value
// is just "Grep" — a tool name kept as-is in every language, the same
// way "SFTP"/"FTP" aren't translated either) falls back to English, per
// Application.T's resolution order.
var Ukrainian = Graphite.Catalog{
	KeyErrorTitle: " Помилка ",
	KeyCancel:     "Скасувати",
	KeyName:       "Назва:",

	KeyMenuFile:    "Файл",
	KeyMenuMark:    "Виділення",
	KeyMenuView:    "Вигляд",
	KeyMenuTab:     "Вкладка",
	KeyMenuNetwork: "Мережа",
	KeyMenuHelp:    "Довідка",

	KeyMenuFileInfo:   "Інформація про файл   F1",
	KeyMenuFileRename: "Перейменувати         F2",
	KeyMenuFileFind:   "Знайти                F3",
	KeyMenuFileCopy:   "Копіювати             F5",
	KeyMenuFileMove:   "Перемістити           F6",
	KeyMenuFileNew:    "Новий файл",
	KeyMenuFileMkdir:  "Нова папка            F7",
	KeyMenuFileDelete: "Видалити              F8",
	KeyMenuFileQuit:   "Вийти                F10",

	KeyMenuMarkToggle: "Позначити/зняти    Ins",
	KeyMenuMarkAll:    "Виділити все",
	KeyMenuMarkNone:   "Зняти виділення",
	KeyMenuMarkInvert: "Інвертувати виділення",

	KeyMenuViewSortName: "Сортувати за назвою",
	KeyMenuViewSortExt:  "Сортувати за розширенням",
	KeyMenuViewSortSize: "Сортувати за розміром",
	KeyMenuViewSortDate: "Сортувати за датою",
	KeyMenuViewRefresh:  "Оновити",

	KeyMenuTabAdd:         "Додати",
	KeyMenuTabAddFileList: "Новий список файлів",
	KeyMenuTabAddTerminal: "Новий термінал",
	KeyMenuTabPinUnpin:    "Закріпити/відкріпити вкладку",
	KeyMenuTabClose:       "Закрити вкладку",
	KeyMenuTabManage:      "Керування вкладками...",

	KeyMenuNetworkCreate:     "Створити з'єднання...",
	KeyMenuNetworkReconnect:  "Перепідключитися",
	KeyMenuNetworkDisconnect: "Відключитися",

	KeyMenuHelpLanguage: "Мова",
	KeyMenuHelpAbout:    "Про програму",

	KeyFKeyInfo:   "Інфо",
	KeyFKeyRename: "Перейм.",
	KeyFKeyFind:   "Пошук",
	KeyFKeyCopy:   "Копіювати %s",
	KeyFKeyMove:   "Перемістити %s",
	KeyFKeyMkdir:  "Тека",
	KeyFKeyDelete: "Видалити",
	KeyFKeyMenu:   "Меню",
	KeyFKeyQuit:   "Вихід",

	KeyDirectionLeft:  "ліворуч",
	KeyDirectionRight: "праворуч",

	KeyGutterCopy: "Копіювати",
	KeyGutterMove: "Перемістити",

	KeyNavRoot: "Корінь",

	KeyStatusNoTagged:    "Файли не позначено",
	KeyStatusTagged:      "Позначено: %d об'єкт(ів), %s",
	KeyStatusServer:      "Сервер: %s",
	KeyStatusServerLocal: "Локально",
	KeyStatusDiskNA:      "Диск: н/д",
	KeyStatusDiskUsage:   "Диск: вільно %s з %s (використано %.0f%%)",
	KeyStatusSearch:      "Пошук: %s",

	KeyFilePaneColName: "Назва",
	KeyFilePaneColExt:  "Розш.",
	KeyFilePaneColSize: "Розмір",
	KeyFilePaneColDate: "Дата",
	KeyFilePaneColAttr: "Атр.",

	KeyFilePaneStatusPlain:  "файлів: %d, тек: %d",
	KeyFilePaneStatusTagged: "файлів: %d, тек: %d — позначено: %d (%s)",

	KeyTabTerminal: "Термінал",
	KeyTabRoot:     "Корінь",

	KeyAboutTagline1: "Кросплатформний двопанельний файловий менеджер,",
	KeyAboutTagline2: "з віддаленими з'єднаннями SFTP та FTP/FTPS.",
	KeyAboutTagline3: "Має функції архівування та створений",
	KeyAboutTagline4: "на легкому TUI-фреймворку Graphite.",
	KeyAboutBeta:     "Бета-версія.",
	KeyClose:         "Закрити",

	KeyFileInfoTitle:    " Інформація про файл ",
	KeyFileInfoMulti:    "Вибрано об'єктів: %d\n\nЗагальний розмір: %s",
	KeyFileInfoName:     "Назва:          %s",
	KeyFileInfoType:     "Тип:            %s",
	KeyFileInfoSize:     "Розмір:         %s",
	KeyFileInfoPerms:    "Права доступу:  %s",
	KeyFileInfoModified: "Змінено:        %s",
	KeyFileInfoAccessed: "Відкрито:       %s",
	KeyFileInfoCreated:  "Створено:       %s",
	KeyFileInfoNotAvail: "недоступно в цій файловій системі",
	KeyFileInfoKindFile: "Файл",
	KeyFileInfoKindDir:  "Директорія",

	KeyFindTitle:        " Пошук файлів ",
	KeyFindResultsTitle: " Пошук файлів ",
	KeySearchIn:         "Пошук у: ",
	KeyNameMask:         "Маска імені: ",
	KeySearchSubfolders: "Шукати в підпапках",
	KeyCaseSensitive:    "Враховувати регістр",
	KeySearch:           "Пошук",
	KeySearching:        "Пошук…",
	KeySearchFound:      "Знайдено: %d",
	KeySearchingFound:   "Пошук… знайдено: %d",

	KeyGrepPattern: "Шаблон: ",
	KeyGrepRegex:   "Регулярний вираз",

	KeyChooseRootTitle: " Виберіть корінь ",

	KeyCopyTitle: " Копіювання ",
	KeyMoveTitle: " Переміщення ",

	KeyArchiveName:      "Ім'я архіву:",
	KeyArchiveOverwrite: "%s вже існує. Перезаписати?",
	KeyNotAnArchive:     "Нерозпізнаний формат архіву: %s",

	KeyConflictTitle:     " Конфлікт ",
	KeyConflictExists:    "Вже існує:",
	KeyConflictApplyAll:  "Застосувати до всіх",
	KeyConflictOverwrite: "Перезаписати",
	KeyConflictSkip:      "Пропустити",
	KeyConflictRename:    "Перейменувати",

	KeyManageTabsTitle:    " Керування вкладками ",
	KeyManageTabsLeft:     "Ліва панель",
	KeyManageTabsRight:    "Права панель",
	KeyManageTabsAddFiles: "+ Файли",
	KeyManageTabsAddTerm:  "+ Терм",
	KeyManageTabsCloseTab: "Закрити",
	KeyManageTabsDone:     "Готово",

	KeyGotoFolderTitle: " Перейти до папки ",
	KeyPath:            "Шлях:",

	KeyRenameTitle: " Перейменувати ",
	KeyNewName:     "Нова назва:",

	KeyNewFileTitle:   " Новий файл ",
	KeyNewFolderTitle: " Нова папка ",
	KeyAlreadyExists:  "Файл або папка з назвою «%s» вже існують.",

	KeyQuitTitle:     " Вихід ",
	KeyQuitMessage:   "Вийти з Diskette?",
	KeyDeleteTitle:   " Видалення ",
	KeyDeleteMessage: "Видалити %d об'єкт(ів)?",

	KeyErrRemoteEditUnsupported: "Відкриття віддаленого файлу поки не підтримується.",

	KeyConnectTitle:      " Підключення до сервера ",
	KeyConnectProtocol:   "Протокол:",
	KeyConnectAuthMethod: "Метод автентифікації:",
	KeyConnectSecurity:   "Безпека:",
	KeyConnectHost:       "Хост:        ",
	KeyConnectPort:       "Порт:        ",
	KeyConnectUsername:   "Користувач:  ",
	KeyConnectKeyFile:    "Файл ключа:  ",
	KeyConnectPassphrase: "Парольна фраза:",
	KeyConnectPassword:   "Пароль:      ",
	KeyConnectButton:     "Підключитися",

	KeyAuthPassword:   "Пароль",
	KeyAuthPrivateKey: "Приватний ключ",
	KeyAuthAgent:      "SSH-агент",

	KeyTLSNone:     "Немає",
	KeyTLSExplicit: "FTPS (явний)",
	KeyTLSImplicit: "FTPS (неявний)",

	KeyErrPortNumber:   "Порт має бути числом.",
	KeyErrHostEmpty:    "Хост не може бути порожнім.",
	KeyErrNotConnected: "Активна вкладка не підключена до сервера.",

	KeyHostKeyTitle:   " Невідомий хост ",
	KeyHostKeyMessage: "Достовірність хоста «%s» неможливо підтвердити.\nВідбиток ключа %s:\n%s\n\nДовіряти цьому ключу та продовжити підключення?",
	KeyHostKeyTrust:   "Довіряти",

	KeyCLIHelpIntro:    "diskette: ця оболонка запущена у вкладці-терміналі diskette, тому команда «diskette» тут звертається до вже запущеного екземпляра, а не відкриває новий.",
	KeyCLIHelpCommands: "Доступні команди:",
	KeyCLIHelpView:     "  diskette view [шлях]        перейти в іншій панелі до шляху (типово: сюди)",
	KeyCLIHelpTag:      "  diskette tag <назва...>     позначити об'єкти за назвою в іншій панелі",
	KeyCLIHelpUntag:    "  diskette untag <назва...>   зняти позначення за назвою в іншій панелі",
	KeyCLIHelpSelect:   "  diskette select <назва>     перемістити курсор до об'єкта в іншій панелі",
	KeyCLIHelpSync:     "  diskette sync on|off        синхронізувати шлях іншої панелі з $PWD",
}
