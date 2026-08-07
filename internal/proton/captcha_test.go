package proton

import (
	"net/url"
	"strings"
	"testing"
)

func captchaURLFor(t *testing.T, base string) *url.URL {
	t.Helper()
	raw, err := New(Options{BaseURL: base}).CaptchaURL("challenge-token")
	if err != nil {
		t.Fatalf("CaptchaURL for base %q: %v", base, err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("CaptchaURL returned an unparseable url %q: %v", raw, err)
	}
	return u
}

// The page must be served by the API subdomain. The web app host answers the
// same path with the same HTML but a Content-Security-Policy that carries no
// nonce, so every inline script on the page is refused and the window renders
// empty.
func TestCaptchaURLUsesTheAPISubdomain(t *testing.T) {
	for _, tc := range []struct {
		base     string
		wantHost string
		wantPath string
	}{
		{"https://mail.proton.me/api", "mail-api.proton.me", "/core/v4/captcha"},
		{"https://account.proton.me/api", "account-api.proton.me", "/core/v4/captcha"},
		{"https://mail.protonmail.ch/api", "mail-api.protonmail.ch", "/core/v4/captcha"},
		// Already naming the API directly: rewriting again would produce
		// mail-api-api.proton.me.
		{"https://mail-api.proton.me", "mail-api.proton.me", "/core/v4/captcha"},
	} {
		u := captchaURLFor(t, tc.base)
		if u.Host != tc.wantHost {
			t.Errorf("base %q: host = %q, want %q", tc.base, u.Host, tc.wantHost)
		}
		if u.Path != tc.wantPath {
			t.Errorf("base %q: path = %q, want %q", tc.base, u.Path, tc.wantPath)
		}
	}
}

// A local, DoH or bare-IP API has no subdomain to move to, so Proton addresses
// it through the /api path prefix instead.
func TestCaptchaURLFallsBackToThePathPrefix(t *testing.T) {
	for _, tc := range []struct {
		base     string
		wantHost string
	}{
		{"http://localhost:8080", "localhost:8080"},
		{"http://localhost", "localhost"},
		{"https://127.0.0.1:8443", "127.0.0.1:8443"},
		{"https://ec2-1-2-3-4.compute.amazonaws.com", "ec2-1-2-3-4.compute.amazonaws.com"},
		{"http://[::1]:8080", "[::1]:8080"},
	} {
		u := captchaURLFor(t, tc.base)
		if u.Host != tc.wantHost {
			t.Errorf("base %q: host = %q, want it unchanged (%q)", tc.base, u.Host, tc.wantHost)
		}
		if u.Path != "/api/core/v4/captcha" {
			t.Errorf("base %q: path = %q, want the /api prefix", tc.base, u.Path)
		}
	}
}

// Both parameters are load-bearing. Without ForceWebMessaging the page looks for
// a native message handler no plain webview provides, and reports nothing.
func TestCaptchaURLCarriesTheChallengeAndForcesWebMessaging(t *testing.T) {
	u := captchaURLFor(t, "https://mail.proton.me/api")
	if got := u.Query().Get("Token"); got != "challenge-token" {
		t.Errorf("Token = %q, want the challenge", got)
	}
	if got := u.Query().Get("ForceWebMessaging"); got != "1" {
		t.Errorf("ForceWebMessaging = %q, want 1", got)
	}
}

// --api-url has to reach the CAPTCHA too. Verifying against production while
// talking to another host fails, and fails invisibly.
func TestCaptchaURLFollowsACustomAPI(t *testing.T) {
	u := captchaURLFor(t, "https://mail.proton.dev/api")
	if u.Host != "mail-api.proton.dev" {
		t.Errorf("host = %q, want the custom API's subdomain", u.Host)
	}
}

func TestCaptchaURLRejectsAnEmptyChallenge(t *testing.T) {
	if _, err := New(Options{}).CaptchaURL(""); err == nil {
		t.Error("an empty challenge cannot produce a usable page")
	}
}

// The default base must land on the host that actually serves the page, since
// that is what almost every run uses.
func TestCaptchaURLFromTheDefaultBase(t *testing.T) {
	got, err := New(Options{}).CaptchaURL("tok")
	if err != nil {
		t.Fatalf("CaptchaURL: %v", err)
	}
	const want = "https://mail-api.proton.me/core/v4/captcha?"
	if !strings.HasPrefix(got, want) {
		t.Errorf("CaptchaURL() = %q, want it to start with %q", got, want)
	}
}
