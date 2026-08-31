package proton

import (
	"net/url"
	"strings"
	"testing"
)

func challenge(methods ...string) *HumanVerificationError {
	if methods == nil {
		methods = []string{MethodCaptcha}
	}
	return &HumanVerificationError{Token: "challenge-token", Methods: methods}
}

func verifyURLFor(t *testing.T, base string, hv *HumanVerificationError) *url.URL {
	t.Helper()
	raw, err := New(Options{BaseURL: base}).VerifyURL(hv)
	if err != nil {
		t.Fatalf("VerifyURL for base %q: %v", base, err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("VerifyURL returned an unparseable url %q: %v", raw, err)
	}
	return u
}

// The page is a sibling of the API, not a path on it, and whichever host the
// client is pointed at is the one whose verification it has to satisfy.
func TestVerifyURLUsesTheVerifySubdomain(t *testing.T) {
	for _, tc := range []struct{ base, wantHost string }{
		{"https://mail.proton.me/api", "verify.proton.me"},
		{"https://account.proton.me/api", "verify.proton.me"},
		{"https://mail-api.proton.me", "verify.proton.me"},
		{"https://mail.protonmail.ch/api", "verify.protonmail.ch"},
		{"https://mail.proton.dev/api", "verify.proton.dev"},
	} {
		u := verifyURLFor(t, tc.base, challenge())
		if u.Host != tc.wantHost {
			t.Errorf("base %q: host = %q, want %q", tc.base, u.Host, tc.wantHost)
		}
	}
}

// Proton names the page in the refusal, and that answer beats anything derived:
// it is the host that raised the challenge and so the host that will honour it.
func TestVerifyURLPrefersTheHostProtonNamed(t *testing.T) {
	hv := challenge()
	hv.WebURL = "https://verify.proton.black/?methods=captcha,email&token=other&embed=1"
	u := verifyURLFor(t, "https://mail.proton.me/api", hv)
	if u.Host != "verify.proton.black" {
		t.Errorf("host = %q, want the one Proton named", u.Host)
	}
}

// The query is built rather than carried over. `embed=1` makes the page hand the
// answer to a host that is not there instead of redeeming it, and a `methods`
// naming email or SMS offers a tab that leads nowhere.
func TestVerifyURLBuildsItsOwnQuery(t *testing.T) {
	hv := challenge(MethodCaptcha, "email", "sms")
	hv.WebURL = "https://verify.proton.me/?methods=captcha,email,sms&token=stale&embed=1"
	u := verifyURLFor(t, "https://mail.proton.me/api", hv)
	if got := u.Query().Get("token"); got != "challenge-token" {
		t.Errorf("token = %q, want the challenge", got)
	}
	if got := u.Query().Get("methods"); got != MethodCaptcha {
		t.Errorf("methods = %q, want %q alone", got, MethodCaptcha)
	}
	if u.Query().Has("embed") {
		t.Error("embed survived, which stops the page redeeming the answer")
	}
}

// A challenge with no CAPTCHA cannot be finished by any client, so no address is
// offered for it - the page would check the code and report it to nobody.
func TestVerifyURLRefusesAChallengeNobodyCanFinish(t *testing.T) {
	_, err := New(Options{}).VerifyURL(challenge("email", "sms"))
	if err == nil {
		t.Fatal("an email-or-sms challenge has no page worth opening")
	}
	for _, want := range []string{"email", "sms"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to name %q", err, want)
		}
	}
}

// A local, DoH or bare-IP API is not a name under a root domain, so it has no
// verify sibling. Sending somebody to a guessed host would waste the challenge.
func TestVerifyURLRefusesAHostWithNoVerifySibling(t *testing.T) {
	for _, base := range []string{
		"http://localhost:8080",
		"http://localhost",
		"https://127.0.0.1:8443",
		"https://ec2-1-2-3-4.compute.amazonaws.com",
		"http://[::1]:8080",
	} {
		if _, err := New(Options{BaseURL: base}).VerifyURL(challenge()); err == nil {
			t.Errorf("base %q: want a refusal, got an address", base)
		}
	}
}

func TestVerifyURLRejectsAnEmptyChallenge(t *testing.T) {
	if _, err := New(Options{}).VerifyURL(&HumanVerificationError{Methods: []string{MethodCaptcha}}); err == nil {
		t.Error("an empty challenge cannot produce a usable page")
	}
	if _, err := New(Options{}).VerifyURL(nil); err == nil {
		t.Error("no challenge cannot produce a usable page")
	}
}

// The default base must land on the host that actually serves the page, since
// that is what almost every run uses.
func TestVerifyURLFromTheDefaultBase(t *testing.T) {
	got, err := New(Options{}).VerifyURL(challenge())
	if err != nil {
		t.Fatalf("VerifyURL: %v", err)
	}
	const want = "https://verify.proton.me/?methods=captcha&token=challenge-token"
	if got != want {
		t.Errorf("VerifyURL() = %q, want %q", got, want)
	}
}
