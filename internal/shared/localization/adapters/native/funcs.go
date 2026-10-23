// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package native

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"raven/internal/shared/localization/domain"
	"raven/internal/shared/localization/ports"
)

// Files supplies the bundled native catalogs.
func Files() embed.FS { return catalogs }

// New validates both native catalogs before returning a message adapter.
func New(source ports.LocaleSource, files fs.FS) (*Messages, error) {
	bundle, err := loadBundle(files)
	if err != nil {
		return nil, fmt.Errorf("initialize native messages: %w", err)
	}

	return &Messages{operations: nativeOperations(source, bundle)}, nil
}

func nativeOperations(
	source ports.LocaleSource,
	bundle *i18n.Bundle,
) map[string]func(string, int, func()) nativeResult {
	return map[string]func(string, int, func()) nativeResult{
		textOperation:      messageOperation(source, bundle),
		subscribeOperation: subscriptionOperation(source),
		refreshOperation:   refreshLanguageOperation(source),
	}
}

func messageOperation(
	source ports.LocaleSource,
	bundle *i18n.Bundle,
) func(string, int, func()) nativeResult {
	return func(key string, count int, _ func()) nativeResult {
		return nativeResult{
			text: render(
				bundle,
				&renderRequest{locale: source.Current().Locale, key: key, count: count},
			),
			cancel: nil,
		}
	}
}

func subscriptionOperation(source ports.LocaleSource) func(string, int, func()) nativeResult {
	return func(_ string, _ int, listener func()) nativeResult {
		return nativeResult{text: "", cancel: source.Subscribe(listener)}
	}
}

func refreshLanguageOperation(source ports.LocaleSource) func(string, int, func()) nativeResult {
	return func(_ string, _ int, _ func()) nativeResult {
		source.Refresh()

		return nativeResult{text: "", cancel: nil}
	}
}

func loadBundle(files fs.FS) (*i18n.Bundle, error) {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	names := []string{"en.json", "ar.json"}
	for index := range names {
		err := loadCatalog(bundle, files, names[index])
		if err != nil {
			return nil, fmt.Errorf("load native catalogs: %w", err)
		}
	}

	return bundle, nil
}

func loadCatalog(bundle *i18n.Bundle, files fs.FS, name string) error {
	file, err := bundle.LoadMessageFileFS(files, name)
	if err != nil {
		return fmt.Errorf("load native catalog %s: %w", name, err)
	}

	return errors.Join(validateCatalog(file))
}

func render(bundle *i18n.Bundle, request *renderRequest) string {
	locale, key, count := request.locale, request.key, request.count
	config := messageOptions(locale, key, count)
	text, err := i18n.NewLocalizer(bundle, locale, domain.English).Localize(config)

	if err != nil && text == "" {
		return nativeFallback(bundle, locale, key)
	}

	return text
}

func messageOptions(locale, key string, count int) *i18n.LocalizeConfig {
	return &i18n.LocalizeConfig{
		MessageID:   key,
		PluralCount: count,
		TemplateData: map[string]any{
			"Count": message.NewPrinter(language.Make(locale)).Sprint(count),
		},
		DefaultMessage: nil,
		Funcs:          nil,
		TemplateParser: nil,
	}
}

// Text formats a native message against the current language.
func (messages *Messages) Text(key string, count int) string {
	return messages.operations[textOperation](key, count, nil).text
}

// Subscribe observes committed language changes.
func (messages *Messages) Subscribe(listener func()) func() {
	return messages.operations[subscribeOperation]("", defaultCount, listener).cancel
}

// Refresh follows changed OS language preferences.
func (messages *Messages) Refresh() { messages.operations[refreshOperation]("", defaultCount, nil) }

func validateCatalog(file *i18n.MessageFile) error {
	if file == nil {
		return errMissingCatalog
	}

	return nil
}

func nativeFallback(bundle *i18n.Bundle, locale, key string) string {
	if key == fallbackKey {
		return "Raven could not complete that action. Please try again."
	}

	return render(bundle, &renderRequest{locale: locale, key: fallbackKey, count: defaultCount})
}
