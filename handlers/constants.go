package handlers

// Message types
const (
	msgStart                    = "Привет! Я бот для публикации мемов в канал. Отправь мне фото или текст, и я опубликую его в канале."
	msgCaptionPrompt            = "Пожалуйста, введите текст подписи для следующих фотографий. Это заменит любую существующую подпись."
	msgCaptionSaved             = "Подпись сохранена! Все фотографии, которые вы отправите, будут использовать эту подпись. Используйте /caption снова, чтобы изменить её."
	msgCaptionOverwrite         = "Предыдущая подпись была заменена на новую."
	msgPostSuccess              = "Пост успешно опубликован в канале!"
	msgPhotoSuccess             = "Фото успешно опубликовано в канале!"
	msgPhotoWithCaption         = "Фото с подписью успешно опубликовано в канале!"
	msgHelpFooter               = "\nЧтобы создать пост, просто отправьте любое текстовое сообщение.\nЧтобы добавить подпись к фото, используйте команду /caption, а затем отправьте фото."
	msgNoCaptionSet             = "Нет активной подписи. Используйте /caption, чтобы установить её."
	msgCurrentCaption           = "Текущая активная подпись:\n%s"
	msgCaptionCleared           = "Активная подпись очищена."
	msgErrorSendingMessage      = "Ошибка отправки сообщения: %s"
	msgErrorCopyingMessage      = "Ошибка копирования сообщения: %s"
	msgNoAdminRights            = "У вас нет прав для публикации сообщений в канале."
	msgNoAdminRightsPhoto       = "У вас нет прав для публикации фотографий в канале."
	msgNoAdminRightsMedia       = "У вас нет прав для публикации медиа групп в канале."
	msgNoAdminRightsCaption     = "У вас нет прав для управления подписями в канале."
	msgNoAdminRightsViewCaption = "У вас нет прав для просмотра подписей в канале."
	msgMediaGroupSuccess        = "Медиа группа опубликована в канале"
	msgMediaGroupWithCaption    = "Медиа группа с подписью опубликована в канале"
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
