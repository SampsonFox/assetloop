package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
)

// TestPasswordMinimumLengthFormsAndSubmissions covers the eight-character
// password minimum end to end: the setup and member forms advertise it, the
// shared application validator rejects shorter input, and eight characters are
// accepted and usable for login.
func TestPasswordMinimumLengthFormsAndSubmissions(t *testing.T) {
	handler := newTestHandler(t)

	setupPage := request(t, handler, http.MethodGet, "/setup", nil, nil)
	csrf := responseCookie(t, setupPage, csrfCookie)
	const setupPasswordInput = `<input name="password" type="password" required minlength="8" autocomplete="new-password">`
	if setupPage.Code != http.StatusOK || !strings.Contains(setupPage.Body.String(), setupPasswordInput) {
		t.Fatalf("setup password field must require 8 characters: status=%d body=%s", setupPage.Code, setupPage.Body.String())
	}

	shortSetup := request(t, handler, http.MethodPost, "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"Length Tenant"}, "base_currency": {"CNY"},
		"username": {"owner"}, "password": {"1234567"},
	}, []*http.Cookie{csrf})
	if shortSetup.Code != http.StatusUnprocessableEntity || !strings.Contains(shortSetup.Body.String(), "至少需要 8 个字符") {
		t.Fatalf("seven-character setup password must be rejected: status=%d body=%s", shortSetup.Code, shortSetup.Body.String())
	}

	setup := request(t, handler, http.MethodPost, "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"Length Tenant"}, "base_currency": {"CNY"},
		"username": {"owner"}, "password": {"abcdefgh"},
	}, []*http.Cookie{csrf})
	if setup.Code != http.StatusSeeOther {
		t.Fatalf("eight-character setup password must be accepted: status=%d body=%s", setup.Code, setup.Body.String())
	}
	session := responseCookie(t, setup, sessionCookie)

	login := request(t, handler, http.MethodPost, "/login", url.Values{
		"csrf_token": {csrf.Value}, "username": {"owner"}, "password": {"abcdefgh"},
	}, []*http.Cookie{csrf})
	if login.Code != http.StatusSeeOther {
		t.Fatalf("setup account must be able to log in: status=%d body=%s", login.Code, login.Body.String())
	}

	shortLogin := request(t, handler, http.MethodPost, "/login", url.Values{
		"csrf_token": {csrf.Value}, "username": {"owner"}, "password": {"abcdefg"},
	}, []*http.Cookie{csrf})
	if shortLogin.Code != http.StatusUnauthorized {
		t.Fatalf("login must evaluate credentials instead of enforcing a length minimum: status=%d body=%s", shortLogin.Code, shortLogin.Body.String())
	}

	members := request(t, handler, http.MethodGet, "/admin/members", nil, []*http.Cookie{session, csrf})
	const memberPasswordInput = `<input name="password" type="password" minlength="8" required autocomplete="new-password">`
	if members.Code != http.StatusOK || !strings.Contains(members.Body.String(), memberPasswordInput) {
		t.Fatalf("member password field must require 8 characters: status=%d body=%s", members.Code, members.Body.String())
	}

	shortMember := request(t, handler, http.MethodPost, "/admin/members", url.Values{
		"csrf_token": {csrf.Value}, "username": {"editor"}, "password": {"1234567"}, "role": {"editor"},
	}, []*http.Cookie{session, csrf})
	if shortMember.Code != http.StatusUnprocessableEntity || !strings.Contains(shortMember.Body.String(), "至少需要 8 个字符") {
		t.Fatalf("seven-character member password must be rejected: status=%d body=%s", shortMember.Code, shortMember.Body.String())
	}

	member := request(t, handler, http.MethodPost, "/admin/members", url.Values{
		"csrf_token": {csrf.Value}, "username": {"editor"}, "password": {"member88"}, "role": {"editor"},
	}, []*http.Cookie{session, csrf})
	if member.Code != http.StatusSeeOther {
		t.Fatalf("eight-character member password must be accepted: status=%d body=%s", member.Code, member.Body.String())
	}

	memberLogin := request(t, handler, http.MethodPost, "/login", url.Values{
		"csrf_token": {csrf.Value}, "username": {"editor"}, "password": {"member88"},
	}, []*http.Cookie{csrf})
	if memberLogin.Code != http.StatusSeeOther {
		t.Fatalf("new member must be able to log in: status=%d body=%s", memberLogin.Code, memberLogin.Body.String())
	}

	for _, locale := range []application.Locale{application.LocaleZhCN, application.LocaleEn} {
		message := textFor(locale, "validation.password_length")
		if !strings.Contains(message, "8") || strings.Contains(message, "12") {
			t.Fatalf("password length message for %s must state 8 characters: %q", locale, message)
		}
	}
}
