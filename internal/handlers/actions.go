package handlers

// Action types for logging and user updates
const (
	ActionCommandStart            = "command_start"
	ActionCommandHelp             = "command_help"
	ActionCommandStatus           = "command_status"
	ActionCommandVersion          = "command_version"
	ActionCommandCaption          = "command_caption"
	ActionCommandShowCaption      = "command_show_caption"
	ActionCommandClearCaption     = "command_clear_caption"
	ActionCommandSuggest          = "command_suggest"
	ActionCommandReview           = "command_review"
	ActionSetCaptionReply         = "set_caption_reply"
	ActionSendTextToChannel       = "send_text_to_channel"
	ActionSendPhotoToChannel      = "send_photo_to_channel"
	ActionSendVideoToChannel      = "send_video_to_channel"
	ActionSendMediaGroupToChannel = "send_media_group_to_channel"
	ActionSuggestMedia            = "suggest_media"
	ActionReviewAction            = "review_action"
	ActionCommandFeedback         = "command_feedback"
	ActionSendFeedback            = "send_feedback"
)

// Utility function to send a success message.
