// Command seed brings the two test accounts to the state the integration suite
// and the README panel both expect.
//
//	go run ./scripts/seed                     # both accounts
//	go run ./scripts/seed --profile primary   # one
//	go run ./scripts/seed --stage             # and make the panel's mail the only unread
//	go run ./scripts/seed --login             # sign in and stop
//
// Every datum is judged against the fixture before it is touched: absent ones
// are made, wrong ones are replaced. Presence alone is not enough - a message
// filed into Archive by a filter, or a label of the wrong colour, would pass a
// presence check and fail an assertion somewhere far away.
//
// The suite guards whole assertions with a skip when a collection is empty - no
// contacts, no vaults, no calendars - which is what this data is for.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/roman-16/proton-cli/tests/fixture"
)

// The accounts. These variables are this program's own, not the CLI's:
// proton takes an account from a signed-in profile, which signIn establishes.
var accounts = []struct{ profile, userVar, passwordVar string }{
	{"primary", "PROTON_CLI_TEST_PRIMARY_USER", "PROTON_CLI_TEST_PRIMARY_PASSWORD"},
	{"secondary", "PROTON_CLI_TEST_SECONDARY_USER", "PROTON_CLI_TEST_SECONDARY_PASSWORD"},
}

func main() {
	var only, stage, loginOnly = flag.String("profile", "", "act on one profile"), flag.Bool("stage", false, "make the panel's mail the only unread"), flag.Bool("login", false, "sign in and stop")
	flag.Parse()

	if err := requireCredentials(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, a := range accounts {
		if err := signIn(a.profile, os.Getenv(a.userVar), os.Getenv(a.passwordVar)); err != nil {
			fmt.Fprintf(os.Stderr, "sign in as %s: %v\n", a.profile, err)
			os.Exit(1)
		}
	}
	if *loginOnly {
		out, err := run(accounts[0].profile, "account", "profiles", "list")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(out)
		return
	}

	work, err := os.MkdirTemp("", "proton-cli-seed-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err := writeFiles(work); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writePasswordFiles(work); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	r := &report{}
	var wg sync.WaitGroup
	for _, a := range accounts {
		if *only != "" && *only != a.profile {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			address := os.Getenv(a.userVar)
			// The calendar is named before anything else, because the events pinned
			// below are pinned in the calendar that name refers to.
			r.calendar(a.profile)
			// Then everything the account holds, in lanes: a folder has to exist
			// before the file in it, so collections of the same thing keep their
			// order, while a label and a vault have nothing to do with each other.
			lanes := lanesOf(mailbox(work), func(c collection) { r.reconcile(a.profile, c) })
			lanes = append(lanes,
				func() { r.photos(a.profile, work) },
				func() {
					if *stage && a.profile == "primary" {
						r.stage(a.profile, address, work)
						return
					}
					r.mail(a.profile, address, work)
				},
			)
			runLanes(lanes)
			// Last, because a sweep puts things in the trash it then empties.
			r.empty(a.profile)
		}()
	}
	wg.Wait()
	if *only == "" {
		r.across(work)
	}

	switch {
	case len(r.failures) > 0:
		fmt.Printf("made %d, replaced %d, swept %d, %d could not be seeded: %s\n",
			r.made, r.remade, r.swept, len(r.failures), strings.Join(r.failures, ", "))
		os.Exit(1)
	case r.made+r.remade+r.swept == 0:
		fmt.Println("already seeded")
	default:
		fmt.Printf("made %d, replaced %d, swept %d\n", r.made, r.remade, r.swept)
	}
}

// lanesOf groups collections by the thing they hold. Two collections of the same
// thing are reconciled in order, because that is where a dependency between them
// can exist - a folder and the file inside it are both "drive".
func lanesOf(cs []collection, reconcile func(collection)) []func() {
	var order []string
	by := map[string][]collection{}
	for _, c := range cs {
		if _, seen := by[c.what]; !seen {
			order = append(order, c.what)
		}
		by[c.what] = append(by[c.what], c)
	}
	lanes := make([]func(), 0, len(order))
	for _, what := range order {
		lanes = append(lanes, func() {
			for _, c := range by[what] {
				reconcile(c)
			}
		})
	}
	return lanes
}

// seedJobs bounds how much of one account is seeded at once, so filling two
// accounts asks no more of Proton than running the suite does.
const seedJobs = 4

func runLanes(lanes []func()) {
	sem := make(chan struct{}, seedJobs)
	var wg sync.WaitGroup
	for _, lane := range lanes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			lane()
		}()
	}
	wg.Wait()
}

func requireCredentials() error {
	var missing []string
	for _, a := range accounts {
		for _, v := range []string{a.userVar, a.passwordVar} {
			if os.Getenv(v) == "" {
				missing = append(missing, v)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("set %s\n(the primary and secondary accounts - the suite and this both create and delete real data)",
			strings.Join(missing, " "))
	}
	return nil
}

// signIn attaches an account to its profile. Signing in again as the same
// account does nothing, so this costs a read once a session exists.
func signIn(profile, address, password string) error {
	cmd := command(profile, "account", "login", "--user", address, "--password-stdin")
	cmd.Stdin = strings.NewReader(password)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", firstLine(strings.TrimSpace(string(out))))
	}
	return nil
}

func writeFiles(work string) error {
	for name, body := range files {
		if body == "" {
			// A file with some bulk, so a listing shows a size worth reading.
			body = strings.Repeat("proton-cli\n", 4000)
		}
		if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0o600); err != nil {
			return err
		}
	}
	return writePhotos(work)
}

// writePNG draws one image. Generated rather than checked in, so the repository
// carries no binaries.
func writePNG(path string, shade color.RGBA, w, h int) error {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: shade}, image.Point{}, draw.Src)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// writePhotos draws the photos the library is topped up with, and the image the
// suite's inline-attachment fixture embeds. Each one is visibly different from
// the last.
func writePhotos(work string) error {
	shades := []color.RGBA{{R: 0x1b, G: 0x10, B: 0x33, A: 0xff}, {R: 0x6d, G: 0x4a, B: 0xff, A: 0xff}, {R: 0x35, G: 0xb1, B: 0x91, A: 0xff}}
	if err := writePNG(filepath.Join(work, fixture.Attachments.Inline), shades[1], 8, 8); err != nil {
		return err
	}
	for i := 0; i < photoCount; i++ {
		if err := writePNG(filepath.Join(work, fmt.Sprintf("photo-%d.png", i+1)), shades[i%len(shades)], 240, 160); err != nil {
			return err
		}
	}
	return nil
}

// across seeds what only exists between the two accounts: what the sharing,
// cross-account delivery and invitation tests look for.
func (r *report) across(work string) {
	from, to := os.Getenv(accounts[0].userVar), os.Getenv(accounts[1].userVar)
	fmt.Println("between the two")

	found, err := rows("secondary", "mail", "messages", "search", "--subject", "Trip photos", "--folder", "inbox", "--limit", "1")
	switch {
	case err != nil:
		r.fail("mail: primary -> secondary", err)
	case len(found) == 0:
		r.make("primary", "mail: primary -> secondary",
			[]string{"mail", "messages", "send", "--to", to, "--subject", "Trip photos",
				"--body", "Sending the ones that came out.", "--attach", filepath.Join(work, "packing-list.txt")})
	}

	if _, err := run("primary", "drive", "share", "get", "/Documents"); err != nil {
		r.make("primary", "drive: /Documents shared with the secondary",
			[]string{"drive", "share", "add", "/Documents", to, "--edit", "--message", "Have a look"})
	}

	events, err := rows("secondary", "calendar", "events", "list", "--start", today(), "--end", inDays(30))
	switch {
	case err != nil:
		r.fail("calendar: invitation", err)
	default:
		if _, ok := find(events, "title", "Quarterly sync"); !ok {
			r.make("secondary", "calendar: invitation awaiting a response",
				[]string{"calendar", "events", "create", "--title", "Quarterly sync",
					"--start", inDays(5) + "T14:00", "--duration", "30m", "--attendee", from})
		}
	}
}
