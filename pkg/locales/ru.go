package locales

// Constants for user messages (in Russian)
const (
	// General Reply Messages
	MsgErrorGeneral       = "Произошла ошибка. Попробуйте еще раз."
	MsgErrorRequiresAdmin = "Эта команда доступна только администраторам."
	MsgSuccessGeneric     = "Выполнено!"
	MsgPostSentToChannel  = "Пост отправлен в канал."

	// Caption Related Reply Messages
	MsgCaptionSetConfirmation       = "Подпись установлена. Она будет добавлена к следующему фото/видео/группе."
	MsgCaptionOverwriteConfirmation = "Подпись обновлена."
	MsgCaptionAskForInput           = "Введите новую подпись:"
	MsgCaptionClearedConfirmation   = "Подпись очищена."
	MsgCaptionShowCurrent           = "Текущая подпись: `%s`"
	MsgCaptionShowEmpty             = "Активная подпись не установлена."
	MsgCaptionForMediaGroup         = "Подпись '%s' будет использована для этой группы медиа."

	// Suggestion Related Reply Messages
	MsgSuggestRequiresSubscription         = "Чтобы предложить пост, вы должны быть подписчиком канала. Подпишитесь и попробуйте снова."
	MsgSuggestSendContentPrompt            = "Хорошо! Теперь отправьте мне ОДНО сообщение с фото (или несколькими фото в виде медиа-группы, до 10 штук). Текст сообщения будет сохранен как подпись к предложению (видна только администраторам)."
	MsgSuggestAlreadyWaitingForContent     = "Я уже ожидаю от вас сообщение с контентом для предложения. Пожалуйста, отправьте его."
	MsgSuggestErrorCheckingSubscription    = "Не удалось проверить вашу подписку. Попробуйте позже."
	MsgSuggestInternalProcessingError      = "Произошла внутренняя ошибка при обработке вашего запроса. Попробуйте позже."
	MsgSuggestionReceivedConfirmation      = "Спасибо! Ваше предложение принято и будет рассмотрено администрацией."
	MsgSuggestionRequiresPhoto             = "Пожалуйста, отправьте сообщение с фото (или медиа-группой фото). Текст будет использован как подпись."
	MsgSuggestionTooManyPhotosError        = "Вы можете прикрепить не более 10 фото к одному предложению."
	MsgSuggestionMediaGroupPartReceived    = "Получил часть медиа-группы. Ожидаю остальные фото..."
	MsgSuggestionMediaGroupProcessingError = "Ошибка при обработке медиа-группы для предложения."

	// Review Related Reply Messages (Placeholder)
	MsgReviewQueueIsEmpty           = "Очередь предложений пуста."
	MsgReviewCurrentSuggestionIndex = "Предложение %d из %d"
	MsgReviewSuggestionDetails      = "От: %s (@%s, ID: %d)\nПодпись: `%s`"
	MsgReviewActionApproved         = "Предложение принято и опубликовано."
	MsgReviewActionRejected         = "Предложение отклонено."
	MsgReviewErrorDuringPublishing  = "Ошибка при публикации принятого предложения."

	// Old constants that might still be in use (verify and integrate/remove)
	MsgStart      = "Привет! Я бот для публикации мемов в канал. Отправь мне фото или текст, и я опубликую его в канале."
	MsgHelpFooter = `

Отправь мне фото или текст для публикации. Используй /caption [текст] для добавления подписи к следующему сообщению.`
	MsgStatus = `Бот запущен
Канал ID: %d
Текущая подпись: %s`
	MsgVersion               = "Версия бота: %s"
	MsgErrorGenericWithArg   = "Произошла ошибка: %v" // Renamed from msgErrorGeneric
	MsgErrorCaptionNotSet    = "Подпись не установлена."
	MsgErrorNoMediaGroup     = "Не удалось найти группу медиа для этой подписи."
	MsgErrorInternalServer   = "Внутренняя ошибка сервера."
	MsgMediaGroupSuccess     = "Медиа группа опубликована в канале"
	MsgMediaGroupWithCaption = "Медиа группа с подписью опубликована в канале"

	// Command Descriptions
	CmdStartDesc        = "Запускает бота"
	CmdHelpDesc         = "Показывает это сообщение"
	CmdStatusDesc       = "Показывает статус бота и ваш ID"
	CmdVersionDesc      = "Показывает версию бота"
	CmdCaptionDesc      = "Установить или обновить подпись для следующей группы фото"
	CmdShowCaptionDesc  = "Показать текущую активную подпись"
	CmdClearCaptionDesc = "Очистить текущую активную подпись"
	CmdSuggestDesc      = "Предложить пост для публикации"
	CmdReviewDesc       = "[Админ] Просмотреть очередь предложенных постов"
)
