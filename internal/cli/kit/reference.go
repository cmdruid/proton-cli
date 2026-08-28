package kit

import (
	"strings"

	"github.com/spf13/cobra"
)

// The root's command groups: what the tool does to your account, what it does
// to the account itself, and what it does to its own installation.
//
// They are declared here rather than beside the root because they are the top
// level of the published reference as well as of a help screen, and the two have
// to agree about which page a command is on.
const (
	GroupApps    = "apps"
	GroupAccount = "account"
	GroupSelf    = "self"
)

// ReferencePage is the slug of the page a command's full entry is published on.
//
// A top-level command that holds others earns a page, because a page is what a
// collection's worth of commands needs. The handful that act on this machine
// share one, because five commands with a dozen flags between them are a section
// rather than a chapter.
func ReferencePage(c *cobra.Command) string {
	top := c
	for top != nil && top.Parent() != nil && top.Parent().Parent() != nil {
		top = top.Parent()
	}
	if top == nil || top.Parent() == nil {
		return ""
	}
	if top.GroupID == GroupSelf {
		return GroupSelf
	}
	return top.Name()
}

// ReferenceHeading is what a command is called on its page: its path, with the
// page's own name dropped.
//
// The page is already titled `proton mail`, so writing `mail` in front of all
// seventy-seven of its headings is the repetition a reference exists to spare
// somebody. It is empty when the command is the page, which has no heading of
// its own because the page is it.
func ReferenceHeading(c *cobra.Command) string {
	path := commandPath(c)
	page := ReferencePage(c)
	if path == "" || path == page {
		return ""
	}
	return strings.TrimPrefix(path, page+" ")
}

// ReferenceAnchor is where a command's heading is linked to.
//
// Both GitHub and the site slugify a heading by hyphenating its words, so the
// heading is the anchor, which is what lets one string serve a link in a page
// and a line on a help screen.
func ReferenceAnchor(c *cobra.Command) string {
	return strings.ReplaceAll(ReferenceHeading(c), " ", "-")
}

// Reference is where a command is documented in full.
func Reference(c *cobra.Command) string {
	page := ReferencePage(c)
	if page == "" {
		return Docs + "/commands/"
	}
	url := Docs + "/commands/" + page + "/"
	if anchor := ReferenceAnchor(c); anchor != "" {
		url += "#" + anchor
	}
	return url
}

// commandPath is the invocation with the program name dropped, which is how
// every command is written once a screen has already said which program it is.
func commandPath(c *cobra.Command) string {
	return strings.TrimPrefix(c.CommandPath(), Program+" ")
}

// Synopsis is the whole command line, the way a manual page opens: the path,
// then whatever arguments the command declares for itself.
func Synopsis(c *cobra.Command) string {
	path := commandPath(c)
	if args := strings.Fields(c.Use); len(args) > 1 {
		path += " " + strings.Join(args[1:], " ")
	}
	return Program + " " + path
}
