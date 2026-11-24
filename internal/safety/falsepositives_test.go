package safety

import "testing"

// M6 of the fifth review: reads that used to need an approval, and plain
// settings in front of a command, which is judged by what follows them.
func TestReadsAndPlainSettingsAreNotDangerous(t *testing.T) {
	for cmd, want := range map[string]Risk{
		`crontab -l`:                                 Safe,
		`crontab -u luis -l`:                         Safe,
		`crontab -e`:                                 Dangerous,
		`crontab /tmp/x`:                             Dangerous,
		`gh auth status`:                             Safe,
		`gh auth status --hostname github.com`:       Safe,
		`gh auth status --show-token`:                Dangerous,
		`gh auth status -t`:                          Dangerous,
		`env FOO=1 brew upgrade`:                     Normal,
		`FOO=1 brew upgrade`:                         Normal,
		`env FOO=1 ls`:                               Normal,
		`env HOMEBREW_NO_AUTO_UPDATE=1 brew upgrade`: Normal,
		`env LC_ALL=C ls`:                            Safe,
		`env PATH=/tmp/x ls`:                         Dangerous,
		`env GIT_SSH_COMMAND=x git status`:           Dangerous,
		`FOO=$(id) ls`:                               Dangerous,
		`env LD_PRELOAD=/tmp/x.so ls`:                Dangerous,
		`env FOO=/tmp/bin ls`:                        Dangerous,
		`env FOO=1 rm -rf /tmp/x`:                    Dangerous,
	} {
		if got := Classify(cmd); got != want {
			t.Errorf("Classify(%q) = %v, want %v", cmd, got, want)
		}
	}
}
