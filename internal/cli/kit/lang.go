// Package kit is the vocabulary every command is built from: the verbs it may
// use, the argument names it may take, the way it selects what to act on, and
// the shared flag groups.
//
// It exists so that the language is declared in one place instead of emerging
// from whatever the neighbouring command happened to do. A new command that
// wants a word not listed here is a command that is inventing one, and the
// conformance test says so.
package kit

// Program is the command, and Alias is its second name: the same binary under
// the project's name, so a line written either way runs.
//
// Every screen speaks Program, whichever name was typed to get there. A help
// screen that renamed itself to match the invocation would teach two languages
// and pin nothing - the examples, the generated reference and the golden files
// all rest on the program being nameable.
const (
	Program = "proton"
	Alias   = "proton-cli"
)

// Verbs is every word that may end a command path.
//
// Each entry is the one word for its idea. Where two words competed, the winner
// is the one Proton's own interface uses; where Proton has no word, the winner is
// the one that reads as ordinary English.
var Verbs = map[string]string{
	// Reading
	"list":   "enumerate a collection",
	"get":    "show one thing in full",
	"search": "query the server's index",

	// Writing
	"create": "make a new thing",
	"update": "change fields of an existing thing",
	"set":    "write one setting",

	// Removing
	"delete":  "remove permanently",
	"trash":   "remove reversibly",
	"restore": "undo a removal",
	"empty":   "remove everything from a trash",

	// Moving
	"move": "put into another container",
	"copy": "duplicate into another container",

	// Bytes
	"upload":   "send a local file",
	"download": "write a remote file to disk",
	"export":   "write documents to disk",

	// Mail
	"send":       "deliver a message",
	"reply":      "answer a message",
	"forward":    "pass a message on",
	"unschedule": "cancel a queued send",
	"read":       "mark as read",
	"unread":     "mark as unread",
	"label":      "attach a label",
	"unlabel":    "detach a label",
	"star":       "attach the starred label",
	"unstar":     "detach the starred label",

	// Toggles
	"enable":  "turn on",
	"disable": "turn off",

	// Sharing
	"link":    "create a public link",
	"unlink":  "remove a public link",
	"add":     "put a member into a container",
	"remove":  "take a member out of a container",
	"accept":  "agree to an invitation",
	"decline": "refuse an invitation",

	// Photos
	"favorite":   "mark as a favourite",
	"unfavorite": "unmark as a favourite",

	// Keys
	"pin":   "trust a public key for a contact",
	"unpin": "stop trusting a pinned key",

	// Calendar
	"respond": "reply to an invitation",

	// Session
	"login":  "authenticate and save a session",
	"logout": "discard a saved session",
	"revoke": "invalidate a session server-side",

	// Reference data
	"options": "list the values a choice offers",

	// The tool itself
	"uninstall":  "remove " + Program,
	"version":    "report the build",
	"completion": "emit a shell completion script",
	"api":        "send a raw authenticated request",
}

// Irreversible lists the verbs whose work neither this CLI nor Proton's own
// clients can take back. Every command whose verb is in here stops for a yes,
// which kit.Mutate guarantees structurally from the action it reports.
//
// `uninstall` belongs beside the two removals because it is the strictest case
// of the same thing: afterwards there is no proton left to undo it with.
var Irreversible = map[string]bool{
	"delete": true, "empty": true, "uninstall": true,
}

// Mutating lists the verbs that change state. Every command whose verb is in
// here has to honour --dry-run, which kit.Mutate guarantees structurally.
var Mutating = map[string]bool{
	"create": true, "update": true, "set": true, "delete": true, "trash": true,
	"restore": true, "empty": true, "move": true, "copy": true, "upload": true,
	"send": true, "reply": true, "forward": true, "unschedule": true,
	"read": true, "unread": true, "label": true, "unlabel": true, "star": true,
	"unstar": true, "enable": true, "disable": true, "link": true,
	"unlink": true, "add": true, "remove": true, "accept": true,
	"decline": true, "favorite": true, "unfavorite": true, "pin": true,
	"unpin": true, "respond": true, "login": true, "logout": true,
	"revoke": true, "uninstall": true,
}

// Placeholders is every argument name a usage string may contain.
//
// One name per idea, so `REF` means the same thing in every command that takes
// one: a full ID, an eight-character short ID, or a human handle such as a
// subject, a name, a path or an email address.
var Placeholders = map[string]string{
	"REF":            "a full ID, a short ID, or a human handle",
	"PATH":           "a Drive path that does not exist yet",
	"SRC":            "a local file or directory to read",
	"DEST":           "a Drive folder to write into",
	"EMAIL":          "an email address",
	"NEW_NAME":       "the name to change something to",
	"KEY":            "a setting key",
	"VALUE":          "a setting value",
	"METHOD":         "an HTTP method",
	"ENDPOINT":       "a Proton API path",
	"VERSION":        "a " + Program + " release, as X.Y.Z",
	"ATTACHMENT_REF": "an attachment on the addressed message",
	"REVISION_REF":   "a revision of the addressed file",
	"CONTACT_REF":    "a contact, when the command already addresses something else",
	"PHOTO_REF":      "a photo, when the command already addresses an album",
}
