package calendar

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cmdruid/proton-cli/internal/proton"
)

// An event has to be written against a named time zone, not just an instant.
//
// A weekly 09:00 meeting stored as a UTC instant moves to 08:00 the moment the
// clocks change; the same meeting anchored to Europe/Vienna stays at 09:00, which
// is what everybody involved meant. So every write needs an IANA zone name, and
// Go's time.Local cannot always supply one: with no TZ variable set it answers
// "Local", which is not a name any other calendar client can resolve.

// Anchor resolves the zone an event is written against.
//
// Preference order: what the caller asked for, then the host's zone if it can be
// named, then the zone the account's own calendar settings are drawn in, and
// failing all of that no anchor at all, which stores a plain UTC instant.
func (s *Service) Anchor(ctx context.Context, requested string) (string, error) {
	if requested != "" {
		if _, err := zoneOf(requested); err != nil {
			return "", err
		}
		return requested, nil
	}
	if name := hostZone(); name != "" {
		return name, nil
	}
	return s.accountZone(ctx), nil
}

// hostZone names this machine's zone, or "" when it cannot be named.
//
// The TZ variable is authoritative when it holds a zone name. Otherwise the
// symlink and the file that the platforms which have them keep the name in are
// read directly, because the standard library discards it.
func hostZone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); plausibleZone(tz) {
		return tz
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if _, after, found := strings.Cut(filepath.ToSlash(target), "zoneinfo/"); found && plausibleZone(after) {
			return after
		}
	}
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		if name := strings.TrimSpace(string(data)); plausibleZone(name) {
			return name
		}
	}
	return ""
}

// plausibleZone reports whether a string looks like an IANA zone name, which is
// what rules out "Local" and the POSIX offset forms.
func plausibleZone(name string) bool {
	if name == "UTC" {
		return true
	}
	if !strings.Contains(name, "/") || strings.HasPrefix(name, "/") {
		return false
	}
	_, err := zoneOf(name)
	return err == nil
}

// accountZone is the zone the account's calendar is drawn in, which is the zone
// Proton's own clients write new events against.
//
// It is asked for once per invocation and only when the host cannot name its own
// zone, which is the case on platforms without a zone file - so a Windows user
// still gets a real anchor rather than a bare UTC instant.
func (s *Service) accountZone(ctx context.Context) string {
	s.zoneOnce.Do(func() {
		var r struct {
			CalendarUserSettings struct{ PrimaryTimezone string }
		}
		if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/settings/calendar"}, &r); err != nil {
			return
		}
		if name := r.CalendarUserSettings.PrimaryTimezone; plausibleZone(name) {
			s.zone = name
		}
	})
	return s.zone
}

// zoneCache is embedded in Service.
type zoneCache struct {
	zoneOnce sync.Once
	zone     string
}
