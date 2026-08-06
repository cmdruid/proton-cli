package proton

import (
	"fmt"
	"net/url"
	"strings"
)

// captchaPath is the endpoint serving the CAPTCHA page.
const captchaPath = "/core/v4/captcha"

// dohDomain is the one domain Proton treats as DNS-over-HTTPS, where the API is
// reached through a path prefix rather than a subdomain.
const dohDomain = ".compute.amazonaws.com"

// CaptchaURL returns the address of the CAPTCHA page for a human verification
// challenge, ready to be opened in a browser or webview.
//
// The page has to be served by the API's own subdomain - mail-api.proton.me -
// and not by the web app host's /api path prefix. Both return the same HTML, but
// only the subdomain sends a Content-Security-Policy carrying the nonce that the
// page's inline scripts are tagged with. Behind the app host's policy every one
// of those scripts is refused, jQuery never loads, and the window stays blank
// with nothing in it to explain why.
//
// ForceWebMessaging makes the page report its result by postMessage. Without it
// the page assumes it is embedded and looks for a native message handler that a
// plain webview does not provide.
//
// Mirrors getApiSubdomainUrl in WebClients (packages/shared/lib/helpers/url.ts).
func (c *Client) CaptchaURL(challenge string) (string, error) {
	if challenge == "" {
		return "", fmt.Errorf("captcha url: empty challenge token")
	}
	u, err := url.Parse(c.base)
	if err != nil {
		return "", fmt.Errorf("captcha url: unusable api base %q: %w", c.base, err)
	}

	// A local, DoH or bare-IP API has no subdomain to move to, so the path
	// prefix is the only way to address it.
	origin := u.Scheme + "://" + u.Host
	if u.Hostname() == "localhost" || strings.HasSuffix(origin, dohDomain) || looksLikeIP(u.Hostname()) {
		u.Path = "/api" + captchaPath
	} else {
		u.Host = apiHost(u.Host)
		u.Path = captchaPath
	}

	u.RawQuery = url.Values{
		"ForceWebMessaging": {"1"},
		"Token":             {challenge},
	}.Encode()
	return u.String(), nil
}

// apiHost moves a host to its API sibling: mail.proton.me becomes
// mail-api.proton.me.
//
// A host already in that form is left alone. WebClients never needs this because
// it starts from the web app's own origin, whereas an --api-url may already name
// the API directly.
func apiHost(host string) string {
	dot := strings.Index(host, ".")
	if dot < 0 {
		return host
	}
	first, rest := host[:dot], host[dot+1:]
	if strings.HasSuffix(first, "-api") {
		return host
	}
	return first + "-api." + rest
}

// looksLikeIP reports whether a hostname is probably an address rather than a
// name, by Proton's own rule: no top-level domain ends in a digit, and an IPv6
// hostname contains a colon.
func looksLikeIP(hostname string) bool {
	if strings.Contains(hostname, ":") {
		return true
	}
	if hostname == "" {
		return false
	}
	last := hostname[len(hostname)-1]
	return last >= '0' && last <= '9'
}
