package handlers

// Constants for user messages (in Russian)
const (
	// Command messages
	msgStart      = "Привет! Я бот для публикации мемов в канал. Отправь мне фото или текст, и я опубликую его в канале."
	msgHelpFooter = "\\n\\nОтправь мне фото или текст для публикации. Используй /caption [текст] для добавления подписи к следующему сообщению."
	msgStatus     = "Бот запущен\\nКанал ID: %d\\nТекущая подпись: %s"
	msgVersion    = "Версия бота: %s"

	// Caption command messages
	msgCaptionPrompt       = "Пожалуйста, введите текст новой подписи."
	msgCaptionSet          = "Подпись установлена. Она будет добавлена к следующему медиа."
	msgCaptionCleared      = "Подпись очищена."
	msgShowCaptionActive   = "Текущая подпись: %s"
	msgShowCaptionInactive = "Нет активной подписи."

	// Success messages
	msgCaptionOverwrite      = "Предыдущая подпись была заменена на новую."
	msgPostSent              = "Сообщение отправлено в канал."
	msgMediaGroupSuccess     = "Медиа группа опубликована в канале"
	msgMediaGroupWithCaption = "Медиа группа с подписью опубликована в канале"

	// Error messages
	msgErrorGeneric       = "Произошла ошибка: %v"
	msgErrorUserNotAdmin  = "У вас нет прав для выполнения этой команды."
	msgErrorCaptionNotSet = "Подпись не установлена."
	msgErrorNoMediaGroup  = "Не удалось найти группу медиа для этой подписи."
	msgErrorInternal      = "Внутренняя ошибка сервера."
)

// Command descriptions
const (
	cmdStartDesc        = "Запустить бота"
	cmdHelpDesc         = "Показать справку"
	cmdStatusDesc       = "Показать статус бота"
	cmdVersionDesc      = "Показать версию бота"
	cmdCaptionDesc      = "Установить подпись для фотографий"
	cmdShowCaptionDesc  = "Показать текущую подпись"
	cmdClearCaptionDesc = "Очистить текущую подпись"
)
