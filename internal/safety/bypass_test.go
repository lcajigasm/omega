package safety

import "testing"

// Commands that used to pass as read-only and ran with no question. Each
// one writes, deletes or runs something else.
func TestNoReadOnlyBypass(t *testing.T) {
	for _, cmd := range []string{
		// Escapes the splitter did not read as the shell does.
		`echo \"; touch /tmp/pwn; echo \"`,
		`echo $'\''; touch /tmp/pwn; echo $'\''`,
		`echo a\;b`,
		// zsh runs these.
		`cat =(touch /tmp/pwn)`,
		`ls /tmp/*(e:'touch /tmp/pwn':)`,
		`(cd /tmp; ls)`,
		// Options that write a file or run a program.
		`rg --pre rm keep /tmp/dir`,
		`rg --pre=rm keep /tmp/dir`,
		`sort -o /tmp/file /dev/null`,
		`sort -uo /tmp/file /dev/null`,
		`sort --output=/tmp/file /dev/null`,
		`uniq /dev/null /tmp/file`,
		`find /etc/hosts -fprint /tmp/out`,
		`find . -fls out`,
		`git branch -D main`,
		`git branch new-branch`,
		`git branch -m old new`,
		`git log --output=/tmp/file -1`,
		`go env -w GOPROXY=https://evil.example`,
		`brew tap evil/repo`,
		`date -s 2020-01-01`,
		`hostname evil`,
		`sysctl -w kern.maxfiles=1`,
		// env that runs a string, and assignments that change what runs.
		`env -S"dd if=/dev/null of=/tmp/file"`,
		`env -C /tmp ls`,
		`env PATH=/tmp/evil ls`,
		`PATH=/tmp/evil ls`,
		`GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.fsmonitor GIT_CONFIG_VALUE_0=touch git status`,
		`GIT_EXTERNAL_DIFF=/tmp/x git log -p`,
		`PAGER=/tmp/x git log`,
		// An option's value taken for the subcommand.
		`systemctl -p status poweroff`,
		`loginctl -p list-sessions terminate-user me`,
		`hostnamectl -H status set-hostname x`,
		// PowerShell: only the first word said "read".
		`Get-ChildItem C:\Users\me\Documents | ForEach-Object { Remove-Item $_.FullName -Recurse -Force }`,
		`Get-ChildItem C:\Users\me | % { $_.Delete() }`,
		`Write-Output (Remove-Item C:\Users\me\Documents\x -Recurse -Force)`,
		`Get-Process | ForEach-Object { Stop-Process $_ -Force }`,
		`Get-ChildItem | ForEach-Object { Set-Content -Path C:\a.txt -Value hi }`,
		`Get-Item x | Where-Object { [IO.File]::Delete($_.FullName) }`,
	} {
		if r := Classify(cmd); r < Dangerous {
			t.Errorf("%s: %s, want dangerous or blocked", cmd, r)
		}
	}
}

// What the catalogue and the model use every day stays read-only.
func TestStillReadOnly(t *testing.T) {
	for _, cmd := range []string{
		`ls -la ~/Library/Caches`,
		`grep "foo(bar)" file.txt`,
		`echo "a; b"`,
		`LC_ALL=C sort file.txt`,
		`sort -u file.txt | uniq -c`,
		`uniq -c file.txt`,
		`env`,
		`env | grep ^HOMEBREW_`,
		`env LC_ALL=C ls`,
		`git branch`,
		`git branch -a`,
		`git branch -vv`,
		`git branch --merged main`,
		`git log --oneline -5`,
		`go env GOPATH`,
		`brew tap`,
		`date`,
		`date +%s`,
		`hostname`,
		`sysctl -n hw.memsize`,
		`rg TODO src`,
		`find . -name '*.log' -size +10M`,
		`systemctl status docker`,
		`systemctl --no-pager status docker`,
		`systemctl --user list-units --type=service`,
		`du -sh ~/Library/Caches 2>/dev/null`,
		`Get-ChildItem C:\Users\me`,
		`Get-ChildItem | Where-Object { $_.Length -gt 1MB }`,
		`Get-Process | Sort-Object CPU | Select-Object -First 5`,
		`Get-ChildItem | ForEach-Object { $_.Name }`,
	} {
		if r := Classify(cmd); r != Safe {
			t.Errorf("%s: %s, want read-only", cmd, r)
		}
	}
	if r := Classify(`HOMEBREW_NO_AUTO_UPDATE=1 brew upgrade`); r != Normal {
		t.Errorf("a harmless variable changed the risk of brew upgrade: %s", r)
	}
}

// Credential files reached through a pattern, a relative path or a flag
// cluster are blocked like the plain forms.
func TestSecretReadsBlocked(t *testing.T) {
	old := getwd
	getwd = func() (string, error) { return "/Users/omega", nil }
	defer func() { getwd = old }()
	for _, cmd := range []string{
		`cat ~/.[s]sh/i[d]_ed25519`,
		`cat ~/.s*/id_*`,
		`cat ~/.ssh/*`,
		`cat ~/.ssh/github_wor?`,
		`cat ~/.aws/credential?`,
		`cat ~/.{ssh,aws}/id_rsa`,
		`cat .ssh/github_work`,
		`cat ~/.git-credentials`,
		`cat ~/.docker/config.json`,
		`cat ~/.config/gcloud/application_default_credentials.json`,
		`security find-generic-password -s omega-cli -wa anthropic`,
		`security find-generic-password -ga anthropic`,
		`"$SUDO_ASKPASS" x`,
		`rg --pre "$SUDO_ASKPASS" . /etc/hosts`,
		`nc -U /tmp/omega-sudo-1/askpass.sock`,
	} {
		if r := Classify(cmd); r != Blocked {
			t.Errorf("%s: %s, want blocked", cmd, r)
		}
	}
	for _, cmd := range []string{
		`cat ~/.ssh/*.pub`,
		`cat ~/.ssh/config`,
		`ls ~/.ssh`,
		`ls ~/Documents/*.txt`,
		`cat ~/.zshrc`,
		`security find-generic-password -s omega-cli -a anthropic`,
	} {
		if r := Classify(cmd); r == Blocked {
			t.Errorf("%s is blocked", cmd)
		}
	}
}
