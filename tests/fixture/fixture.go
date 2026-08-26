// Package fixture declares the mail the integration suite reads.
//
// Most mail tests need *a* real, delivered, encrypted message of a particular
// shape rather than a freshly sent one. Sending those shapes on every run costs
// two things: a delivery to wait for, and a message from a sending allowance that
// a free plan caps at fifty an hour. Spending four of them on fixtures is what
// decides how often the suite can be run, so they live on the account instead and
// `scripts/seed` puts them there once.
//
// Declaring them here rather than in either place is what keeps the two in step:
// the seed creates exactly what the suite looks for, because both read this.
//
// A test that MUTATES a message (mark, star, move, trash) or exercises the send
// path itself must not use these - it sends its own.
package fixture

// Mail is one message the suite expects to find in the inbox.
type Mail struct {
	// Subject identifies it. It reads like somebody's mail rather than like a
	// fixture, for the same reason the rest of the seed does, and it never carries
	// the `proton-cli-test-` prefix, which belongs to what the suite makes and
	// clears up itself.
	Subject string
	// Body is sent verbatim.
	Body string
	// HTML sends the body as HTML, which an inline image needs.
	HTML bool
	// Attach and Inline name files in the seed's work directory.
	Attach string
	Inline string
}

// Plain is a message with nothing special about it: no attachments, and a body
// carrying no quote markers, so stripping quotes from it must change nothing.
//
// Its body contains its subject, which is what lets a test tell a body it can
// read from a body it only thinks it can read.
var Plain = Mail{
	Subject: "Notes from the reading group",
	Body: "Notes from the reading group are below.\n\n" +
		"We finished the second chapter and agreed to meet on Thursday.\n",
}

// Quoted carries the canonical reply block, which is the thing --strip-quotes
// removes. The wording is the fixture's whole purpose, so it is spelled out
// rather than generated.
var Quoted = Mail{
	Subject: "Re: the second chapter",
	Body: "My new note for the reading group.\n\n" +
		"On Tue, 24 Sep 2024, Sender <a@b.com> wrote:\n\n" +
		"> ancient quoted text\n" +
		"> that should disappear\n",
}

// Attachments carries one regular attachment and one inline image, so both the
// attachment tests and the ones about telling the two dispositions apart have a
// real message to read. One message answers both, because a mail with an inline
// image and an attachment is one shape rather than two.
var Attachments = Mail{
	Subject: "Trail photos and the packing list",
	Body:    "<p>Both are attached.</p>",
	HTML:    true,
	Attach:  "packing-list.txt",
	Inline:  "inline-image.png",
}

// Mutable are messages a test may change and change back: marked unread, starred,
// moved, trashed and restored. The change is the subject of those tests, not the
// sending, so they take one of these instead of sending their own.
//
// There are several because tests run at the same time and each needs a message
// nobody else is changing; they are handed out one at a time. They read like
// ordinary mail for the same reason the rest does.
var Mutable = []Mail{
	{Subject: "Library books are due on Friday", Body: "Two of them can be renewed online.\n"},
	{Subject: "Your parcel is on its way", Body: "It should arrive between nine and noon.\n"},
	{Subject: "Team lunch next Tuesday", Body: "The place on the corner, half past twelve.\n"},
	{Subject: "Water meter reading", Body: "The reading for this quarter is recorded.\n"},
}

// All is every message the suite expects, for the seed to reconcile.
func All() []Mail { return append([]Mail{Plain, Quoted, Attachments}, Mutable...) }

// AliasName is the Pass alias the suite reads rather than makes.
//
// Making an alias is the tightest thing the free plan meters - a handful an
// hour, against five tests that each want one - so only the test about creating
// one makes its own. The address behind this is Proton's to choose and differs
// per account, which is why the name is what identifies it.
const AliasName = "Newsletters"
