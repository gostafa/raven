// Gostafa 2026.
// SPDX-License-Identifier: Apache-2.0.

package native

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"raven/internal/shared/localization/domain"
)

type sourceStub struct{ state domain.State }

func (source *sourceStub) Current() domain.State { return source.state }
func (*sourceStub) Subscribe(func()) func()      { return func() {} }
func (*sourceStub) Refresh()                     {}

func TestNativeMessagesFollowLocaleAndArabicPlurals(t *testing.T) {
	t.Parallel()
	source := &sourceStub{state: domain.Resolve("en", nil)}
	messages, err := New(source, Files())
	if err != nil {
		t.Fatal(err)
	}
	if messages.Text("open", 0) != "Open Raven" ||
		!strings.Contains(messages.Text("reminderBody", 1), "item is") {
		t.Fatal("invalid English catalog")
	}
	source.state = domain.Resolve("ar", nil)
	for _, count := range []int{0, 1, 2, 3, 11, 100} {
		body := messages.Text("reminderBody", count)
		if body == "" || strings.Contains(body, "{{") || strings.Contains(body, "items") {
			t.Fatalf("%d: %q", count, body)
		}
	}
	if !strings.Contains(messages.Text("reminderBody", 3), "٣") {
		t.Fatal(messages.Text("reminderBody", 3))
	}
	if messages.Text("startup", 0) != "التشغيل عند تسجيل الدخول" {
		t.Fatal("invalid native menu translation")
	}
}

func TestMissingCatalogsAndUnknownMessages(t *testing.T) {
	t.Parallel()
	source := &sourceStub{state: domain.Resolve("ar", nil)}
	if _, err := New(source, fstest.MapFS{}); err == nil {
		t.Fatal("missing catalogs succeeded")
	}
	messages, err := New(source, Files())
	if err != nil {
		t.Fatal(err)
	}
	if messages.Text("unknown", 0) != messages.Text("actionFailed", 0) {
		t.Fatal("missing fallback")
	}
	messages.Subscribe(func() {})()
	messages.Refresh()
	if nativeFallback(i18n.NewBundle(language.English), "ar", fallbackKey) == "" {
		t.Fatal("missing terminal fallback")
	}
	if validateCatalog(nil) == nil || validateCatalog(&i18n.MessageFile{}) != nil {
		t.Fatal("invalid catalog validation")
	}
}
