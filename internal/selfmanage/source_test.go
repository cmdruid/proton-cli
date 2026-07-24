package selfmanage

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		path string
		want Kind
	}{
		{"/home/alice/.local/bin/proton-cli", KindStandalone},
		{"/usr/local/bin/proton-cli", KindStandalone},
		{"/usr/bin/proton-cli", KindStandalone},
		{"/nix/store/abc123-proton-cli-1.9.11/bin/proton-cli", KindNix},
		{"/opt/homebrew/Caskroom/proton-cli/1.9.11/proton-cli", KindHomebrew},
		{"/usr/local/Cellar/proton-cli/1.9.11/bin/proton-cli", KindHomebrew},
		{"/home/linuxbrew/.linuxbrew/bin/proton-cli", KindHomebrew},
		{"/home/alice/project/node_modules/@roman-16/proton-cli-linux-x64/bin/proton-cli", KindNpm},
		{`C:\Users\alice\AppData\Local\Microsoft\WinGet\Packages\Roman-16.ProtonCLI_x\proton-cli.exe`, KindWinget},
		{`C:\Users\alice\AppData\Local\Programs\proton-cli\proton-cli.exe`, KindStandalone},
	}
	for _, tc := range cases {
		if got := Classify(tc.path); got != tc.want {
			t.Errorf("Classify(%q) = %d, want %d", tc.path, got, tc.want)
		}
	}
}
