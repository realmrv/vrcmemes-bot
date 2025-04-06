package locales

import (
	"embed"
	"encoding/json"
	"log"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// DefaultLanguage defines the default language code used by the bot.
const DefaultLanguage = "ru"

//go:embed *.json
var localeFS embed.FS

var bundle *i18n.Bundle

// Init initializes the i18n bundle by loading language files.
func Init() {
	bundle = i18n.NewBundle(language.Russian) // Use Russian as the bundle's default language
	// Register the unmarshal function for JSON files
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	// Load translation files from the embedded filesystem
	// Read the current directory embedded in localeFS
	fs, err := localeFS.ReadDir(".")
	if err != nil {
		log.Fatalf("Failed to read embedded locales directory: %v", err)
	}

	loadedFiles := 0
	for _, file := range fs {
		// Check if it's a JSON file
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			filePath := file.Name() // Path is relative to the embed root
			_, err := bundle.LoadMessageFileFS(localeFS, filePath)
			if err != nil {
				log.Printf("WARN: Failed to load message file '%s': %v", filePath, err)
			} else {
				log.Printf("Successfully loaded message file: %s", filePath)
				loadedFiles++
			}
		}
	}
	if loadedFiles == 0 {
		log.Fatalf("No message files loaded from locales/")
	}
	log.Printf("i18n bundle initialized with %d file(s).", loadedFiles)
}

// NewLocalizer creates a localizer for the given language preferences.
// It takes language tags (e.g., "en", "ru") or Accept-Language header string.
func NewLocalizer(langPrefs ...string) *i18n.Localizer {
	if bundle == nil {
		log.Println("WARN: i18n bundle not initialized when creating localizer. Calling Init().")
		Init()
	}
	return i18n.NewLocalizer(bundle, langPrefs...)
}

// GetMessage retrieves and formats a message by its ID using the provided localizer.
// msgID: The ID of the message (e.g., "MsgWelcome").
// templateData: Optional map for template variables (e.g., map[string]interface{}{"Name": "User"}).
// pluralCount: Optional pointer to an int for pluralization rules.
func GetMessage(localizer *i18n.Localizer, msgID string, templateData map[string]interface{}, pluralCount *int) string {
	config := &i18n.LocalizeConfig{
		MessageID:    msgID,
		TemplateData: templateData,
	}
	// Add plural count if provided
	if pluralCount != nil {
		config.PluralCount = *pluralCount
	}

	localizedMsg, err := localizer.Localize(config)
	if err != nil {
		// Fallback or error logging
		// Log the failed message ID. Getting the specific attempted language from localizer isn't straightforward.
		log.Printf("ERROR: Failed to localize message ID '%s': %v. Falling back to English.", msgID, err)

		// Create a localizer specifically for English fallback
		englishLocalizer := i18n.NewLocalizer(bundle, language.English.String())
		fallbackMsg, fallbackErr := englishLocalizer.Localize(config)
		if fallbackErr == nil {
			// If English translation is found, return it
			return fallbackMsg
		}

		// If English also fails, log and return the message ID
		log.Printf("ERROR: Failed to localize message ID '%s' in English fallback as well. Returning ID.", msgID)
		return msgID // Return the ID as the ultimate fallback
	}
	return localizedMsg
}
