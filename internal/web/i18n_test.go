package web

import (
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
)

func TestLocalePacksHaveIdenticalKeys(t *testing.T) {
	zh := messages[application.LocaleZhCN]
	en := messages[application.LocaleEn]
	if len(zh) != len(en) {
		t.Fatalf("locale pack size differs: zh-CN=%d en=%d", len(zh), len(en))
	}
	for key := range zh {
		if en[key] == "" {
			t.Errorf("English locale is missing %q", key)
		}
	}
	for key := range en {
		if zh[key] == "" {
			t.Errorf("zh-CN locale is missing %q", key)
		}
	}
}

func TestLocaleMatchingAndChineseFallback(t *testing.T) {
	if got := matchLocale("en-US,en;q=0.9,zh-CN;q=0.8"); got != application.LocaleEn {
		t.Fatalf("English browser locale matched %q", got)
	}
	if got := matchLocale("fr-FR,zh-CN;q=0.8"); got != application.LocaleZhCN {
		t.Fatalf("unsupported browser locale did not fall back to zh-CN: %q", got)
	}
	delete(messages[application.LocaleEn], "test.fallback")
	messages[application.LocaleZhCN]["test.fallback"] = "中文回退"
	t.Cleanup(func() { delete(messages[application.LocaleZhCN], "test.fallback") })
	if got := textFor(application.LocaleEn, "test.fallback"); got != "中文回退" {
		t.Fatalf("missing English key fallback=%q", got)
	}
}

func TestSafeReturnToRejectsExternalTargets(t *testing.T) {
	for _, value := range []string{"https://example.com", "//example.com/path", "javascript:alert(1)", "relative"} {
		if _, ok := safeReturnTo(value, "/"); ok {
			t.Errorf("unsafe return target %q was accepted", value)
		}
	}
	if got, ok := safeReturnTo("/assets?id=1", "/"); !ok || got != "/assets?id=1" {
		t.Fatalf("safe return target: got=%q ok=%v", got, ok)
	}
}
