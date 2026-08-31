// Command gendocs writes the command reference from the command tree itself.
//
// The reference is generated because prose drifts and a tree does not. Every
// command gets its whole entry - what it does, how it is invoked, what it takes,
// and the examples it already carries - so the answer to "what does this flag
// do" exists somewhere other than a terminal. CI regenerates it and fails on a
// diff, so a command that exists is a command that is documented, and one that
// was renamed cannot keep its old name here.
//
// Where a command is published is kit's answer, not this file's: the same
// function tells a help screen which URL to print, so a heading and a link
// cannot disagree.
//
// Prose here is written as whole paragraphs on one line, as the hand-written
// pages are: a hard wrap is a decision about somebody else's window.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/roman-16/proton-cli/internal/cli"
	"github.com/roman-16/proton-cli/internal/cli/kit"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const dir = "docs/commands"

func main() {
	root := cli.Root()
	pages := collect(root)

	// Emptied first, so a page for an app that no longer exists goes with it.
	// The directory is generated whole or not at all.
	out := filepath.Join(moduleRoot(), dir)
	if err := os.RemoveAll(out); err != nil {
		fail(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		fail(err)
	}
	for slug, commands := range pages {
		if err := write(slug+".md", page(root, slug, commands)); err != nil {
			fail(err)
		}
	}
	if err := write("README.md", index(root, pages)); err != nil {
		fail(err)
	}
}

// collect files every documented command under the page it belongs on.
func collect(root *cobra.Command) map[string][]*cobra.Command {
	pages := map[string][]*cobra.Command{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Hidden || c.Name() == "help" {
			return
		}
		if c != root {
			pages[kit.ReferencePage(c)] = append(pages[kit.ReferencePage(c)], c)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	for slug := range pages {
		sort.Slice(pages[slug], func(i, j int) bool {
			return pages[slug][i].CommandPath() < pages[slug][j].CommandPath()
		})
	}
	return pages
}

// index is the way in: the grammar, the flags that work everywhere, and what
// each page holds.
func index(root *cobra.Command, pages map[string][]*cobra.Command) string {
	var b strings.Builder
	b.WriteString("# Command reference\n\n")
	b.WriteString("Every command, every argument and every flag, generated from the command tree.\n\n")
	b.WriteString("```\n" + kit.Program + " <app> <collection> <verb> [TARGET...] [--flags]\n```\n\n")
	b.WriteString("Anywhere a command shows `REF`, you can pass a full ID, the eight-character short ID a list printed, or something human: a subject, a name, a path, an email address. See [Naming the thing you want](../language.md#naming-the-thing-you-want).\n")

	for _, group := range root.Groups() {
		fmt.Fprintf(&b, "\n## %s\n\n", strings.TrimSuffix(group.Title, ":"))
		for _, top := range sorted(root.Commands()) {
			if top.GroupID != group.ID || top.Hidden {
				continue
			}
			slug := kit.ReferencePage(top)
			if group.ID == kit.GroupSelf {
				continue
			}
			fmt.Fprintf(&b, "- **[`%s %s`](%s.md)** - %s. %s.\n",
				kit.Program, top.Name(), slug, lower(top.Short), tally(pages[slug]))
		}
		if group.ID == kit.GroupSelf {
			fmt.Fprintf(&b, "- **[`%s` itself](%s.md)** - updating, uninstalling, completions and what a release changed. %s.\n",
				kit.Program, kit.GroupSelf, tally(pages[kit.GroupSelf]))
		}
	}
	b.WriteString("\n")

	b.WriteString("## Flags that work on every command\n\n")
	b.WriteString("These are declared on the root, so they can be given to any command and mean the same thing on all of them.\n\n")
	writeFlags(&b, root.LocalFlags())
	b.WriteString("\nSee [Configuration](../configuration.md) for what each one changes, and [Output](../output.md) for the exit codes.\n")
	return b.String()
}

// page is one page of the reference: every command filed under it, in tree
// order, each with everything the tree knows about it.
//
// The command the page is named for has no entry of its own, because the page is
// its entry: its description is the lead, and whatever it holds is the list
// under it. Only its body - a synopsis, examples, flags - is written out, and
// only when it has one.
func page(root *cobra.Command, slug string, commands []*cobra.Command) string {
	var b strings.Builder

	if slug == kit.GroupSelf {
		fmt.Fprintf(&b, "# %s itself\n\n", kit.Program)
		b.WriteString("Updating, uninstalling, shell completions, and what a release changed.\n\n")
		b.WriteString("These act on this installation rather than on your account, so none of them needs you to be signed in.\n")
		for _, c := range commands {
			writeCommand(&b, c)
		}
		b.WriteString(footer)
		return b.String()
	}

	// The title carries no backticks: it becomes frontmatter, which is read as a
	// string rather than as markdown, so a backtick there is a backtick on screen.
	top := child(root, slug)
	fmt.Fprintf(&b, "# %s %s\n\n%s\n\n", kit.Program, slug, lead(top))
	fmt.Fprintf(&b, "Every command under `%s %s`, with the arguments and flags it takes. For these commands in use, see [the guide](../apps/%s.md).\n",
		kit.Program, slug, slug)
	writeBody(&b, top)

	for _, c := range commands {
		if c == top {
			continue
		}
		writeCommand(&b, c)
	}

	b.WriteString(footer)
	return b.String()
}

const footer = "\n---\n\nEvery command also takes the [flags that work everywhere](README.md#flags-that-work-on-every-command).\n"

// writeCommand is one command's entry, and the shape never varies: what it is,
// what it holds, how it is invoked, what it takes, and it being used.
//
// A collection is a heading and everything under it is a level down, so the
// page's own contents list reads as the tree it documents rather than as one
// flat run of seventy-seven lines. It stops at three, which is as deep as a
// contents list is read.
func writeCommand(b *strings.Builder, c *cobra.Command) {
	heading := kit.ReferenceHeading(c)
	level := "###"
	if !strings.Contains(heading, " ") {
		level = "##"
	}
	// A heading here is a command line, and reads as one. The backticks cost
	// nothing: both GitHub and the site drop them when they slugify, so the
	// anchor kit hands a help screen still lands on it.
	fmt.Fprintf(b, "\n%s `%s`\n\n%s\n", level, heading, lead(c))
	writeBody(b, c)
}

// writeBody is everything below a command's description.
func writeBody(b *strings.Builder, c *cobra.Command) {
	if subs := visible(c); len(subs) > 0 {
		names := make([]string, len(subs))
		for i, sub := range subs {
			names[i] = "`" + sub.Name() + "`"
		}
		fmt.Fprintf(b, "\nHolds %s.\n", list(names))
	}
	if !c.Runnable() {
		return
	}

	fmt.Fprintf(b, "\n```\n%s\n```\n", kit.Synopsis(c))

	if example := strings.TrimSpace(c.Example); example != "" {
		fmt.Fprintf(b, "\n```bash\n%s\n```\n", example)
	}

	if flags := c.LocalFlags(); hasOwnFlags(flags) {
		b.WriteString("\n")
		writeFlags(b, flags)
	}
}

// lead is how a command introduces itself: the long form where it has one, and
// its one-line summary where it has not.
func lead(c *cobra.Command) string {
	if long := strings.TrimSpace(c.Long); long != "" {
		return reflow(long)
	}
	return c.Short + "."
}

// writeFlags is the table a reader scans to find the one they want.
func writeFlags(b *strings.Builder, set *pflag.FlagSet) {
	b.WriteString("| Flag | Description |\n| --- | --- |\n")
	set.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		name := "--" + f.Name
		if f.Shorthand != "" {
			name = "-" + f.Shorthand + ", " + name
		}
		if kind := f.Value.Type(); kind != "bool" {
			name += " " + kind
		}
		usage := f.Usage
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && f.DefValue != "[]" &&
			!strings.Contains(usage, "default") {
			usage += " (default `" + f.DefValue + "`)"
		}
		fmt.Fprintf(b, "| `%s` | %s |\n", name, escape(usage))
	})
}

// reflow joins a paragraph that was hard-wrapped for a terminal, because a
// markdown reader has a window of their own and wrapping it twice reads as a
// list of fragments.
func reflow(text string) string {
	paragraphs := strings.Split(text, "\n\n")
	for i, p := range paragraphs {
		lines := strings.Split(p, "\n")
		// An indented block is laid out on purpose; only prose is joined.
		if strings.HasPrefix(p, "  ") {
			continue
		}
		for j := range lines {
			lines[j] = strings.TrimSpace(lines[j])
		}
		paragraphs[i] = strings.Join(lines, " ")
	}
	return strings.Join(paragraphs, "\n\n")
}

func write(name, body string) error {
	return os.WriteFile(filepath.Join(moduleRoot(), dir, name), []byte(body), 0o644)
}

// moduleRoot is where the repository is, worked out from this file rather than
// from the working directory: a generator that writes wherever it was started
// from writes the reference into somebody's home directory.
func moduleRoot() string {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		fail(fmt.Errorf("cannot locate the generator's own source"))
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(self)))
}

func visible(c *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, sub := range c.Commands() {
		if !sub.Hidden && sub.Name() != "help" {
			out = append(out, sub)
		}
	}
	return sorted(out)
}

func sorted(in []*cobra.Command) []*cobra.Command {
	out := append([]*cobra.Command(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func child(parent *cobra.Command, name string) *cobra.Command {
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	fail(fmt.Errorf("no command named %q", name))
	return nil
}

func hasOwnFlags(set *pflag.FlagSet) bool {
	found := false
	set.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && f.Name != "help" {
			found = true
		}
	})
	return found
}

// tally counts what a page holds, which is the commands that do work rather than
// the groups holding them: a group is a heading, not something you can run.
func tally(commands []*cobra.Command) string {
	n := 0
	for _, c := range commands {
		if c.Runnable() {
			n++
		}
	}
	if n == 1 {
		return "1 command"
	}
	return fmt.Sprintf("%d commands", n)
}

func list(items []string) string {
	switch len(items) {
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

func lower(s string) string {
	if s == "" {
		return s
	}
	// Only the first word, and only when it is an ordinary one: "Proton" and
	// "Vaults" are not the same case.
	first, rest, _ := strings.Cut(s, " ")
	if strings.ToUpper(first) == first {
		return s
	}
	return strings.ToLower(first[:1]) + first[1:] + ifRest(rest)
}

func ifRest(rest string) string {
	if rest == "" {
		return ""
	}
	return " " + rest
}

func escape(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
