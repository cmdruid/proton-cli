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
)

// The accounts. These variables are this program's own, not the CLI's:
// proton-cli takes an account from a signed-in profile, which signIn establishes.
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

	r := &report{}
	for _, a := range accounts {
		if *only != "" && *only != a.profile {
			continue
		}
		address := os.Getenv(a.userVar)
		fmt.Printf("%s (%s)\n", address, a.profile)
		r.calendar(a.profile)
		for _, c := range mailbox(work) {
			r.reconcile(a.profile, c)
		}
		r.photos(a.profile, work)
		if *stage && a.profile == "primary" {
			r.stage(a.profile, address, work)
		} else {
			r.mail(a.profile, address, work)
		}
		r.empty(a.profile)
	}
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

// writePhotos draws the photos the library is topped up with. Generated rather
// than checked in, so the repository carries no binaries and each one is
// visibly different from the last.
func writePhotos(work string) error {
	shades := []color.RGBA{{R: 0x1b, G: 0x10, B: 0x33, A: 0xff}, {R: 0x6d, G: 0x4a, B: 0xff, A: 0xff}, {R: 0x35, G: 0xb1, B: 0x91, A: 0xff}}
	for i := 0; i < photoCount; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 240, 160))
		draw.Draw(img, img.Bounds(), &image.Uniform{C: shades[i%len(shades)]}, image.Point{}, draw.Src)
		f, err := os.Create(filepath.Join(work, fmt.Sprintf("photo-%d.png", i+1)))
		if err != nil {
			return err
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
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
