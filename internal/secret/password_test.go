package secret

import (
	"strings"
	"testing"
)

// A password asked to include digits that happens to contain none is one that
// will be rejected by whatever asked for digits.
func TestPasswordIncludesEveryClassAskedFor(t *testing.T) {
	for i := 0; i < 200; i++ {
		got, err := Password(Options{Length: 8, Digits: true, Symbols: true, Upper: true})
		if err != nil {
			t.Fatalf("password: %v", err)
		}
		if len(got) != 8 {
			t.Fatalf("length = %d, want 8", len(got))
		}
		for name, class := range map[string]string{
			"lowercase": lower, "uppercase": upper, "a digit": digits, "a symbol": symbols,
		} {
			if !strings.ContainsAny(got, class) {
				t.Fatalf("%q contains no %s", got, name)
			}
		}
	}
}

// What was not asked for does not appear.
func TestPasswordLeavesOutWhatWasNotAskedFor(t *testing.T) {
	for i := 0; i < 200; i++ {
		got, err := Password(Options{Length: 16})
		if err != nil {
			t.Fatalf("password: %v", err)
		}
		if strings.ContainsAny(got, digits) {
			t.Fatalf("%q has a digit and none was asked for", got)
		}
		if strings.ContainsAny(got, symbols) {
			t.Fatalf("%q has a symbol and none was asked for", got)
		}
		if strings.ContainsAny(got, upper) {
			t.Fatalf("%q has a capital and none was asked for", got)
		}
	}
}

// The misread-prone characters are left out of a mixed password and allowed back
// when letters are the only alphabet there is.

func TestPasswordAmbiguousCharacters(t *testing.T) {
	mixed := ""
	for i := 0; i < 200; i++ {
		got, err := Password(Options{Length: 20, Digits: true, Symbols: true, Upper: true})
		if err != nil {
			t.Fatalf("password: %v", err)
		}
		mixed += got
	}
	if strings.ContainsAny(mixed, ambiguous) {
		t.Error("a mixed password should leave out the characters people misread")
	}
}

// Too short to hold one of each is refused rather than silently dropping a class.
func TestPasswordRefusesALengthThatCannotHoldEveryClass(t *testing.T) {
	if _, err := Password(Options{Length: 2, Digits: true, Symbols: true, Upper: true}); err == nil {
		t.Error("a two-character password cannot hold four classes and should say so")
	}
}

// Zero means Proton's default rather than an empty password.
func TestPasswordDefaultsToProtonsLength(t *testing.T) {
	got, err := Password(Options{})
	if err != nil {
		t.Fatalf("password: %v", err)
	}
	if len(got) != DefaultLength {
		t.Errorf("length = %d, want %d", len(got), DefaultLength)
	}
}

// Two passwords in a row are not the same password.
func TestPasswordsDiffer(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		got, err := Password(Options{Length: 20, Digits: true, Symbols: true, Upper: true})
		if err != nil {
			t.Fatalf("password: %v", err)
		}
		if seen[got] {
			t.Fatalf("%q came up twice in 100 draws", got)
		}
		seen[got] = true
	}
}
