package suggestions

import (
	"strings"
)

// escapeMarkdownV2 escapes characters reserved by Telegram MarkdownV2.
func escapeMarkdownV2(s string) string {
	charsToEscape := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	var result strings.Builder
	result.Grow(len(s))
	escapeChar := '\\'

	for _, r := range s {
		escape := false
		for _, char := range charsToEscape {
			if string(r) == char {
				escape = true
				break
			}
		}
		if escape {
			result.WriteRune(escapeChar)
		}
		result.WriteRune(r)
	}
	return result.String()
}
