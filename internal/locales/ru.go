package locales

import Graphite "github.com/yeoblyv/graphite"

// Russian. Any key not listed here (e.g. KeyMenuFileGrep, whose value is
// just "Grep" — a tool name kept as-is in every language, the same way
// "SFTP"/"FTP" aren't translated either) falls back to English, per
// Application.T's resolution order.
var Russian = Graphite.Catalog{
	KeyErrorTitle: " Ошибка ",
	KeyCancel:     "Отмена",
	KeyName:       "Имя:",

	KeyMenuFile:    "Файл",
	KeyMenuMark:    "Отметить",
	KeyMenuView:    "Вид",
	KeyMenuTab:     "Вкладка",
	KeyMenuNetwork: "Сеть",
	KeyMenuHelp:    "Справка",

	KeyMenuFileInfo:   "Информация о файле    F1",
	KeyMenuFileRename: "Переименовать         F2",
	KeyMenuFileFind:   "Найти                 F3",
	KeyMenuFileCopy:   "Копировать            F5",
	KeyMenuFileMove:   "Переместить           F6",
	KeyMenuFileNew:    "Новый файл",
	KeyMenuFileMkdir:  "Новая папка           F7",
	KeyMenuFileDelete: "Удалить               F8",
	KeyMenuFileQuit:   "Выход                F10",

	KeyMenuMarkToggle: "Отметить/снять     Ins",
	KeyMenuMarkAll:    "Выделить всё",
	KeyMenuMarkNone:   "Снять выделение",
	KeyMenuMarkInvert: "Инвертировать выделение",

	KeyMenuViewSortName: "Сортировать по имени",
	KeyMenuViewSortExt:  "Сортировать по расширению",
	KeyMenuViewSortSize: "Сортировать по размеру",
	KeyMenuViewSortDate: "Сортировать по дате",
	KeyMenuViewRefresh:  "Обновить",

	KeyMenuTabAdd:         "Добавить",
	KeyMenuTabAddFileList: "Новый список файлов",
	KeyMenuTabAddTerminal: "Новый терминал",
	KeyMenuTabPinUnpin:    "Закрепить/открепить вкладку",
	KeyMenuTabClose:       "Закрыть вкладку",
	KeyMenuTabManage:      "Управление вкладками...",

	KeyMenuNetworkCreate:     "Создать подключение...",
	KeyMenuNetworkReconnect:  "Переподключиться",
	KeyMenuNetworkDisconnect: "Отключиться",

	KeyMenuHelpLanguage: "Язык",
	KeyMenuHelpAbout:    "О программе",

	KeyFKeyInfo:   "Инфо",
	KeyFKeyRename: "Перим.",
	KeyFKeyFind:   "Поиск",
	KeyFKeyCopy:   "Копир. %s",
	KeyFKeyMove:   "Перем. %s",
	KeyFKeyMkdir:  "Папка",
	KeyFKeyDelete: "Удалить",
	KeyFKeyMenu:   "Меню",
	KeyFKeyQuit:   "Выход",

	KeyDirectionLeft:  "влево",
	KeyDirectionRight: "вправо",

	KeyGutterCopy: "Копировать",
	KeyGutterMove: "Переместить",

	KeyNavRoot: "Корень",

	KeyStatusNoTagged:    "Файлы не отмечены",
	KeyStatusTagged:      "Отмечено: %d объект(ов), %s",
	KeyStatusServer:      "Сервер: %s",
	KeyStatusServerLocal: "Локально",
	KeyStatusDiskNA:      "Диск: н/д",
	KeyStatusDiskUsage:   "Диск: свободно %s из %s (занято %.0f%%)",
	KeyStatusSearch:      "Поиск: %s",

	KeyFilePaneColName: "Имя",
	KeyFilePaneColExt:  "Расш.",
	KeyFilePaneColSize: "Размер",
	KeyFilePaneColDate: "Дата",
	KeyFilePaneColAttr: "Атр.",

	KeyFilePaneStatusPlain:  "файлов: %d, папок: %d",
	KeyFilePaneStatusTagged: "файлов: %d, папок: %d — отмечено: %d (%s)",

	KeyTabTerminal: "Терминал",
	KeyTabRoot:     "Корень",

	KeyAboutTagline1: "Кроссплатформенный двухпанельный файловый менеджер,",
	KeyAboutTagline2: "с удалёнными подключениями SFTP и FTP/FTPS.",
	KeyAboutTagline3: "Поддерживает архивирование и собран",
	KeyAboutTagline4: "на лёгком TUI-фреймворке Graphite.",
	KeyAboutBeta:     "Бета-версия.",
	KeyClose:         "Закрыть",

	KeyFileInfoTitle:    " Информация о файле ",
	KeyFileInfoMulti:    "Выбрано объектов: %d\n\nОбщий размер: %s",
	KeyFileInfoName:     "Имя:            %s",
	KeyFileInfoType:     "Тип:            %s",
	KeyFileInfoSize:     "Размер:         %s",
	KeyFileInfoPerms:    "Права доступа:  %s",
	KeyFileInfoModified: "Изменён:        %s",
	KeyFileInfoAccessed: "Открыт:         %s",
	KeyFileInfoCreated:  "Создан:         %s",
	KeyFileInfoNotAvail: "недоступно в этой файловой системе",
	KeyFileInfoKindFile: "Файл",
	KeyFileInfoKindDir:  "Каталог",

	KeyFindTitle:        " Поиск файлов ",
	KeyFindResultsTitle: " Поиск файлов ",
	KeySearchIn:         "Искать в: ",
	KeyNameMask:         "Маска имени: ",
	KeySearchSubfolders: "Искать в подпапках",
	KeyCaseSensitive:    "Учитывать регистр",
	KeySearch:           "Поиск",
	KeySearching:        "Поиск…",
	KeySearchFound:      "Найдено: %d",
	KeySearchingFound:   "Поиск… найдено: %d",

	KeyGrepPattern: "Шаблон: ",
	KeyGrepRegex:   "Регулярное выражение",

	KeyChooseRootTitle: " Выберите корень ",

	KeyCopyTitle: " Копирование ",
	KeyMoveTitle: " Перемещение ",

	KeyArchiveName:      "Имя архива:",
	KeyArchiveOverwrite: "%s уже существует. Перезаписать?",
	KeyNotAnArchive:     "Нераспознанный формат архива: %s",

	KeyConflictTitle:     " Конфликт ",
	KeyConflictExists:    "Уже существует:",
	KeyConflictApplyAll:  "Применить ко всем",
	KeyConflictOverwrite: "Перезаписать",
	KeyConflictSkip:      "Пропустить",
	KeyConflictRename:    "Переименовать",

	KeyManageTabsTitle:    " Управление вкладками ",
	KeyManageTabsLeft:     "Левая панель",
	KeyManageTabsRight:    "Правая панель",
	KeyManageTabsAddFiles: "+ Файлы",
	KeyManageTabsAddTerm:  "+ Терм",
	KeyManageTabsCloseTab: "Закрыть",
	KeyManageTabsDone:     "Готово",

	KeyGotoFolderTitle: " Перейти в папку ",
	KeyPath:            "Путь:",

	KeyRenameTitle: " Переименовать ",
	KeyNewName:     "Новое имя:",

	KeyNewFileTitle:   " Новый файл ",
	KeyNewFolderTitle: " Новая папка ",
	KeyAlreadyExists:  "Файл или папка с именем «%s» уже существуют.",

	KeyQuitTitle:     " Выход ",
	KeyQuitMessage:   "Выйти из Diskette?",
	KeyDeleteTitle:   " Удаление ",
	KeyDeleteMessage: "Удалить %d объект(ов)?",

	KeyErrRemoteEditUnsupported: "Открытие удалённого файла пока не поддерживается.",
	KeyErrDestinationInsideSrc:  "Невозможно выполнить: место назначения — та же папка или папка внутри копируемой либо перемещаемой.",

	KeyConnectTitle:      " Подключение к серверу ",
	KeyConnectProtocol:   "Протокол:",
	KeyConnectAuthMethod: "Метод аутентификации:",
	KeyConnectSecurity:   "Безопасность:",
	KeyConnectHost:       "Хост:        ",
	KeyConnectPort:       "Порт:        ",
	KeyConnectUsername:   "Пользователь:",
	KeyConnectKeyFile:    "Файл ключа:  ",
	KeyConnectPassphrase: "Кодовая фраза:",
	KeyConnectPassword:   "Пароль:      ",
	KeyConnectButton:     "Подключиться",

	KeyAuthPassword:   "Пароль",
	KeyAuthPrivateKey: "Приватный ключ",
	KeyAuthAgent:      "SSH-агент",

	KeyTLSNone:     "Нет",
	KeyTLSExplicit: "FTPS (явный)",
	KeyTLSImplicit: "FTPS (неявный)",

	KeyErrPortNumber:   "Порт должен быть числом.",
	KeyErrHostEmpty:    "Хост не может быть пустым.",
	KeyErrNotConnected: "Активная вкладка не подключена к серверу.",

	KeyHostKeyTitle:   " Неизвестный хост ",
	KeyHostKeyMessage: "Подлинность хоста «%s» не может быть подтверждена.\nОтпечаток ключа %s:\n%s\n\nДоверять этому ключу и продолжить подключение?",
	KeyHostKeyTrust:   "Доверять",

	KeyCLIHelpIntro:    "diskette: эта оболочка запущена во вкладке-терминале diskette, поэтому команда «diskette» здесь обращается к уже запущенному экземпляру, а не открывает новый.",
	KeyCLIHelpCommands: "Доступные команды:",
	KeyCLIHelpView:     "  diskette view [путь]        перейти в другой панели к пути (по умолчанию: сюда)",
	KeyCLIHelpTag:      "  diskette tag <имя...>       отметить объекты по имени в другой панели",
	KeyCLIHelpUntag:    "  diskette untag <имя...>     снять отметку по имени в другой панели",
	KeyCLIHelpSelect:   "  diskette select <имя>       переместить курсор к объекту в другой панели",
	KeyCLIHelpSync:     "  diskette sync on|off        синхронизировать путь другой панели с $PWD",
}
