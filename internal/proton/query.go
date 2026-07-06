package proton

import "net/url"

// Query builds url.Values from alternating key/value pairs, e.g.
// Query("Page", "0", "PageSize", "50"). A trailing key without a value is
// ignored. It lives here because building request query parameters is the
// transport layer's concern.
func Query(kv ...string) url.Values {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	return q
}
