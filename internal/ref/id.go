package ref

import "strings"

// What a reference looks like, declared once.
//
// This used to be common knowledge rather than stated knowledge: the ID cache
// had one idea of a complete ID, the leading-dash workaround in the cli layer
// had another, and the renderer that prints references had a third. They agreed
// often enough to look correct, and where they disagreed the CLI printed
// references it would not read back - `pass items list` shows SHARE/ITEM, and a
// share whose ID begins with a dash produced a row nobody could paste.
//
// So the shapes are written down, and they are written down from measurement.
// Every one below was taken from a live account rather than inferred:
//
//	kind                                       length  padding  bytes
//	messages, conversations, labels, folders,      88  "=="        64
//	  addresses, contacts, calendars, events,
//	  Pass shares and items, vaults
//	Drive links, Drive share invitations           22  none        16
//	session UIDs                                   32  none         -
//
// The alphabet is URL-safe base64 throughout, which Proton's own clients rely
// on: Pass routes parse a share out of a path with `share/([^/]+)`, and the
// calendar joins a calendar and an event as `${calendarID}.${eventID}` - neither
// would be safe if an ID could contain a slash or a dot.
//
// Session UIDs are lowercase alphanumeric, so they can never begin with a dash
// and never need protecting from flag parsing. They are recognised all the same,
// so that classifying a reference never depends on which command is asking.

// ShortLen is how many leading characters of an ID identify it in interactive
// output. Eight is short enough to read back off the screen and long enough that
// collisions within one account are vanishingly rare; when two cached IDs do
// collide, resolution reports the ambiguity rather than guessing.
const ShortLen = 8

// The two separators a reference may contain. Neither can occur inside an ID,
// which is what lets a compound reference stay a single pasteable token.
const (
	// Compound joins a thing to the container that addresses it: a Pass item
	// inside its share, an event inside its calendar.
	Compound = "/"
	// Occurrence names one instance of a recurring event, and is not an ID: it
	// is a timestamp, so it is never shortened or matched against the cache.
	Occurrence = "@"
)

// Full reports whether s is a complete ID as Proton issues them.
//
// The padded form is matched by its ending rather than its exact length so that
// a shape this CLI has not seen is still recognised as complete; the two
// unpadded forms have no ending to match and are matched by length.
func Full(s string) bool {
	switch {
	case len(s) >= 60 && strings.HasSuffix(s, "=="):
		return isBase64URL(s)
	case len(s) == 22:
		return isBase64URL(s)
	case len(s) == 32:
		return isLowerAlnum(s)
	}
	return false
}

// Short reports whether s could be the beginning of an ID, and so worth looking
// up in the cache of what has been shown.
//
// The predicate is deliberately loose: plenty of search terms ("Personal",
// "invoice-2024") match it. That costs nothing, because a lookup is only used
// when it hits - a miss falls through to the original text, which the service
// layer then treats as a name to search for.
func Short(s string) bool {
	return len(s) >= ShortLen && !Full(s) && len(s) < 60 &&
		!strings.HasSuffix(s, "==") && isBase64URL(s)
}

// Split takes a reference apart into the IDs it joins and the occurrence it
// names, either of which may be absent.
//
// It is the inverse of Join, and the one place the notation is parsed. A
// reference with no separator comes back as a single part, which is the shape a
// human handle - a subject, a name, a path - also takes.
func Split(reference string) (parts []string, occurrence string) {
	body, occurrence, _ := strings.Cut(reference, Occurrence)
	return strings.Split(body, Compound), occurrence
}

// Join writes the IDs back as the single token a user pastes.
func Join(parts ...string) string { return strings.Join(parts, Compound) }

// Unambiguous reports whether s begins with a dash and is certainly a reference
// whatever command it was passed to.
//
// A complete ID settles it by shape - no run of shorthand flags is 22 or 88
// characters of base64. A separator settles it outright, because no flag or
// cluster of flags contains a slash or an at-sign.
//
// It answers without knowing the command, which is what makes it the right
// question when there is no command to ask about - reporting a parse failure
// after the fact, say. The cli layer can do better than this before the fact,
// because it can ask the command whether the token names flags it actually has;
// a short reference such as "-Qt-s7R_" is unknowable here and decidable there.
func Unambiguous(s string) bool {
	if len(s) < 2 || s[0] != '-' || s[1] == '-' {
		return false
	}
	// The dash is part of the ID rather than a prefix to strip: '-' is one of
	// base64url's sixty-four characters, which is the whole reason this problem
	// exists. A Drive link really is named "-Qt-s7R_oGCru5u3Kv6Y8Q", all
	// twenty-two characters of it.
	if strings.Contains(s, Compound) || strings.Contains(s, Occurrence) {
		return isCompound(s)
	}
	return Full(s)
}

// isCompound reports whether every ID in a compound reference is well formed.
func isCompound(s string) bool {
	parts, _ := Split(s)
	for _, p := range parts {
		if p == "" || !isBase64URL(p) {
			return false
		}
	}
	return true
}

// Plausible reports whether s is shaped like a leading-dash reference at all:
// dash-first, and base64url in every segment.
//
// It is the question to ask when something else will rule out the alternative -
// the cli layer pairs it with "and it names no shorthand this command has" - and
// the question to ask before offering advice about "--", where being generous
// costs nothing.
func Plausible(s string) bool {
	if len(s) < ShortLen || s[0] != '-' || s[1] == '-' {
		return false
	}
	return isCompound(s)
}

func isBase64URL(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '=':
		default:
			return false
		}
	}
	return true
}

func isLowerAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}
