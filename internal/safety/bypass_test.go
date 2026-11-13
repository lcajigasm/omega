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

// Found by the second review: each of these ran with no question.
func TestSecondReviewNotReadOnly(t *testing.T) {
	for _, cmd := range []string{
		// A quote inside a comment swallowed the command on the next line.
		"echo hi # \"\nrm -rf /tmp/dir/victim # \"",
		"ls # it's\nrm -rf /tmp/dir/victim",
		"Get-ChildItem <# a\n#> ; Remove-Item x",
		// The shell removes quotes and expands; the option checks read the text.
		`find /tmp/d -type f "-delete"`,
		`find /tmp/d ${TERM:+-delete}`,
		`find /tmp/d -{delete,delete}`,
		`find . -de''lete`,
		`find . -\delete`,
		`find . '-exec' rm {} +`,
		`find . -[d]elete`,
		`find . $TERM:s/dumb/-delete/`,
		`sort "-o" file`,
		`sort ${TERM:+-o} ~/.zshenv /dev/null`,
		`git log -1 "--output=/tmp/p"`,
		`rg '--pre' sh -l . .`,
		`go env '-w' X=1`,
		`rpm -qa '--pipe' 'touch /tmp/x'`,
		`ps 'eww'`,
		`ps '-E'`,
		`journalctl '--vacuum-size=1K'`,
		`ls {a,b}`,
		`cat file{1..3}`,
		// "help" and "version" as a word after anything.
		`rm foo help`,
		`killall node help`,
		`mv a version`,
		`reboot help`,
		`shutdown -h`,
		`rm --help`,
		// A read-only start with a tail that changes things.
		`git remote -v add evil https://example.com/x`,
		`git remote -v set-url origin https://example.com/x`,
		`git tag v1.0`,
		`softwareupdate -l -i -a -R`,
		`softwareupdate --list --install --all`,
		`mdutil -s -E /`,
		`xcode-select -p --switch /tmp/x`,
		`find . -exec ls {} + -exec rm {} +`,
		`find . -exec sh -c 'rm x' {} +`,
		`awk 'BEGIN{system("touch /tmp/x")}'`,
		`awk '{print > "/tmp/x"}' file`,
		`awk '{print | "sh"}' file`,
		`awk -f prog.awk file`,
		`man -P 'touch /tmp/x' ls`,
		`xattr -d com.apple.quarantine app`,
		`pkgutil --forget com.example.pkg`,
		`networksetup -setairportpower en0 off`,
		`ifconfig en0 down`,
		`plutil -convert xml1 file.plist`,
		`date 010112002020`,
		`date -f %Y 2020`,
		`uniq -f 1 in out`,
		`file -C -m magic`,
		// A file created by a redirection read as a duplicated descriptor.
		`echo hi >&2file`,
		`ls >/dev/nullx`,
		// Commands that never end.
		`tail -f /var/log/system.log`,
		`tail -F /var/log/system.log`,
		`tail -20f /var/log/system.log`,
		`journalctl -f`,
		`journalctl -u ssh --follow`,
		`vm_stat 1`,
		`top -l 0`,
		`cat /dev/zero`,
		`cat /dev/urandom`,
		`wc -l < /dev/random`,
		`log stream`,
		`ping example.com`,
		`docker logs -f web`,
		`docker stats`,
		`netstat -w 1`,
		`lsof -r 1`,
		// The environment printed into something that cuts the values up.
		`env | grep -o .`,
		`printenv | fold -w 3`,
		`env | rev`,
		// Linux package managers and systemctl given another configuration or machine.
		`apt list -o Dir::Bin::dpkg=/tmp/evil`,
		`apt-cache policy -o x=y`,
		`dnf list --setopt=reposdir=/tmp/x`,
		`apt-get -s --no-simulate upgrade -y`,
		`apt-get -s=no upgrade`,
		`pacman -Si -y x`,
		`pacman -Ss -c`,
		`systemctl status x -H root@host`,
		`systemctl --host=evil status`,
		`systemctl -M box status`,
	} {
		if r := Classify(cmd); r < Dangerous {
			t.Errorf("%q: %s, want dangerous or blocked", cmd, r)
		}
	}
}

// Ordinary read-only commands the policy used to ask about, or refuse.
func TestSecondReviewReadOnly(t *testing.T) {
	at(t, "/work/project", "")
	for _, cmd := range []string{
		`ls # just a comment`,
		`ls $HOME/x ${HOME}/y "$TERM"`,
		"ls\t-la",
		`fdesetup status`,
		`nvram -p`,
		`man mkfs`,
		`ls /usr/sbin | grep -i nvram`,
		`which mkfs.ext4`,
		`command -v nvram`,
		`cd ~/proj && git status`,
		`[ -d ~/x ] && echo yes`,
		`test -f x`,
		`awk '{print $1}' file`,
		`awk -F: '$3 > 100 {print $1}' /etc/passwd`,
		`cut -d',' -f1 file`,
		`tr a-z A-Z`,
		`basename /a/b`,
		`dirname /a/b`,
		`readlink /tmp/x`,
		`realpath .`,
		`diff a b`,
		`shasum -a 256 file`,
		`md5 file`,
		`lsof -i :8080`,
		`pgrep -fl node`,
		`ifconfig`,
		`ifconfig en0`,
		`netstat -an`,
		`netstat -anv -p tcp`,
		`plutil -p file.plist`,
		`mdls file`,
		`xattr -l file`,
		`xattr file`,
		`pkgutil --pkgs`,
		`pkgutil --pkg-info com.apple.pkg.Core`,
		`pkgutil --files com.apple.pkg.Core`,
		`scutil --get ComputerName`,
		`networksetup -listallhardwareports`,
		`networksetup -getdnsservers Wi-Fi`,
		`spctl --status`,
		`log show --last 1h --predicate 'eventMessage contains "error"'`,
		`defaults domains`,
		`defaults find com.apple.dock`,
		`defaults read-type com.apple.dock autohide`,
		`git diff`,
		`git diff --stat HEAD~1`,
		`git show HEAD`,
		`git log @{u}..`,
		`git tag`,
		`git tag -l 'v*'`,
		`git tag --contains HEAD`,
		`git remote`,
		`git remote -v`,
		`git remote get-url origin`,
		`docker logs web`,
		`docker logs --tail 50 web`,
		`docker inspect web`,
		`docker stats --no-stream`,
		`brew ls`,
		`brew missing`,
		`brew uses --installed openssl`,
		`brew desc jq`,
		`npm root -g`,
		`npm view left-pad version`,
		`date -Iseconds`,
		`date -r 1700000000`,
		`date -j -f %Y-%m-%d 2024-01-02 +%s`,
		`date -u +%Y`,
		`date -v-1d +%F`,
		`date -d yesterday +%F`,
		`uniq -f 1 file`,
		`find . -name '*.log' -exec ls -la {} +`,
		`find . -type f -exec wc -l {} +`,
		`echo "a -> b"`,
		`echo "--- disk ---"`,
		`git log --format='%an <%ae>'`,
		`jq '.[] | select(.size > 100)' file.json`,
		`echo $((1+2))`,
		`echo hi >&2`,
		`softwareupdate -l`,
		`softwareupdate --list --no-scan`,
		`mdutil -s /`,
		`mdutil -as`,
		`xcode-select -p`,
		`tail -n 20 /var/log/system.log`,
		`journalctl -u ssh -n 50 --no-pager`,
		`vm_stat`,
		`vm_stat -c 2 1`,
		`top -l 1 -n 5`,
		`head -c 16 /dev/urandom | wc -c`,
		`cat /dev/null`,
		`node -v`,
		`python3 -V`,
		`java -version`,
		`cargo --version`,
		`tmux -V`,
		`git help`,
		`brew help cleanup`,
		`brew cleanup --help`,
		`env | sort`,
		`printenv | grep -c HOME`,
		`systemctl status docker -n 50`,
		`systemctl show docker -p ActiveState`,
		`systemctl list-units --type=service --state=failed`,
		`pacman -Si bash`,
		`Get-Process | select Name`,
		`Get-ChildItem | where Length -gt 100`,
		`Get-ChildItem | Sort-Object { $_.Length }`,
	} {
		if r := Classify(cmd); r != Safe {
			t.Errorf("%q: %s, want read-only", cmd, r)
		}
	}
	// Thinning local snapshots frees space: it asks, it is not refused.
	if r := Classify("tmutil deletelocalsnapshots 2024-01-01-000000"); r != Dangerous {
		t.Errorf("tmutil deletelocalsnapshots: %s, want dangerous", r)
	}
}

// What must never run, or never be printed, and slipped through.
func TestSecondReviewBlocked(t *testing.T) {
	at(t, "/work/project", "")
	old := userHome
	userHome = func() string { return "/home/ana" }
	t.Cleanup(func() { userHome = old })
	for _, cmd := range []string{
		// The home directory by other names.
		`rm -rf /home/ana`,
		`rm -rf /home/ana/`,
		`rm -rf ~/../ana`,
		`rm -rf /Users/$USER`,
		`rm -rf /home/${USER}`,
		`rm -rf ${HOME:-/}`,
		`rm -rf ${TMPDIR:-/}`,
		`find /home/ana -delete`,
		`cd ~ && rm -rf *`,
		`cd / && rm -rf *`,
		`cd /; rm -rf .`,
		`cd && rm -rf .*`,
		`cd ~/x && rm -rf ..`,
		// Whole disks.
		`diskutil apfs eraseVolume APFS x disk1s1`,
		`diskutil ap deleteContainer disk1`,
		`diskutil reformat disk2`,
		`newfs_hfs /dev/disk2`,
		`sudo newfs_apfs /dev/disk2`,
		`cp /dev/zero /dev/disk2`,
		`find / -name x -exec mkfs.ext4 {} +`,
		`echo mkfs.ext4 /dev/sda | sh`,
		`nvram boot-args=x`,
		`fdesetup disable`,
		// The sudo helper, by the folder it lives in.
		`cat /tmp/omega-sudo-123/askpass`,
		`ls /tmp/omega-sudo-123`,
		// A credential that stays in the environment, named.
		`echo ${GITHUB_TOKEN:0:10} ${GITHUB_TOKEN:10}`,
		`echo $GITHUB_TOKEN`,
		`printenv GITHUB_TOKEN | grep -o .`,
		`echo $GH_TOKEN | rev`,
		`printenv HOMEBREW_GITHUB_API_TOKEN`,
		`echo $env:GITHUB_TOKEN`,
		// Credential files and histories.
		`cat ~/.zsh_history`,
		`tail -50 ~/.bash_history`,
		`cat ~/.psql_history`,
		`cat ~/.pgpass`,
		`cat ~/.my.cnf`,
		`cat ~/.cargo/credentials.toml`,
		`cat ~/.cargo/credentials`,
		`cat ~/.claude/.credentials.json`,
		`cat ~/.claude.json`,
		`cat ~/.codex/auth.json`,
		`cat ~/.local/share/opencode/auth.json`,
		`cat ~/.config/rclone/rclone.conf`,
		`cat ~/.terraform.d/credentials.tfrc.json`,
		`cat ~/.vault-token`,
		`cat ~/.azure/accessTokens.json`,
		`cat ~/.huggingface/token`,
		`cat ~/.aws/sso/cache/abc.json`,
		`cat ~/.aws/cli/cache/abc.json`,
		`cat ~/.config/gcloud/legacy_credentials/me/adc.json`,
		`cat ~/.kube/cache/token`,
		`cat ~/.gradle/gradle.properties`,
		`cat ~/.m2/settings.xml`,
		`sqlite3 "$HOME/Library/Application Support/Google/Chrome/Default/Cookies" .dump`,
		`strings "$HOME/Library/Application Support/Google/Chrome/Default/Login Data"`,
		`cat ~/Library/Cookies/Cookies.binarycookies`,
		`cat /etc/shadow`,
		`cat ~/.z*_history`,
		`cd ~/.docker && cat config.json`,
		`cd /etc && cat shadow`,
		`find ~/.ssh -type f -exec head {} +`,
		`Get-Content $env:APPDATA\Microsoft\Windows\PowerShell\PSReadLine\ConsoleHost_history.txt`,
		`Get-Clipboard`,
		`pbpaste`,
	} {
		if r := Classify(cmd); r != Blocked {
			t.Errorf("%q: %s, want blocked", cmd, r)
		}
	}
	for _, cmd := range []string{
		`rm -rf /home/ana/Downloads/x`,
		`rm -rf /home/bea`,
		`cd ~/proj && rm -rf *`,
		`cat ~/.zshrc`,
		`cat ~/.zprofile`,
		`ls ~/.kube`,
		`ls ~/Library/Cookies`,
		`mo history`,
		`cat /etc/pam.d/login`,
		`gh auth status`,
		`diskutil apfs list`,
	} {
		if r := Classify(cmd); r == Blocked {
			t.Errorf("%q is blocked", cmd)
		}
	}
	for _, p := range []string{"/home/ana/.zsh_history", "/home/ana/Library/Application Support/Google/Chrome/Default/Login Data",
		"/home/ana/.kube/config", "/home/ana/.claude.json", `C:\Users\ana\AppData\Roaming\Microsoft\Windows\PowerShell\PSReadLine\ConsoleHost_history.txt`} {
		if !SecretFile(p) {
			t.Errorf("SecretFile(%q) must be true: read_file would send it to the model", p)
		}
	}
	for _, p := range []string{"/home/ana/.zshrc", "/home/ana/.bashrc", "/home/ana/.profile", "/home/ana/notes/history.md"} {
		if SecretFile(p) {
			t.Errorf("SecretFile(%q) must stay readable", p)
		}
	}
}

// Docker cleanups always ask, with global options in between and under
// their other spellings.
func TestAlwaysConfirmSecondReview(t *testing.T) {
	for cmd, want := range map[string]bool{
		"docker --context default system prune -af --volumes": true,
		"docker -H unix:///x volume prune -f":                 true,
		"docker compose down -v":                              true,
		"docker compose -f a.yml down --volumes":              true,
		"docker-compose down -v":                              true,
		"docker volume remove x":                              true,
		"docker image remove x":                               true,
		"docker stack rm x":                                   true,
		"podman system prune -af":                             true,
		"nerdctl system prune -af":                            true,
		"sudo docker --log-level debug rmi abc":               true,
		"docker compose down":                                 false,
		"docker compose up -d":                                false,
		"docker --context default ps":                         false,
		"docker logs rm":                                      false,
		"podman ps -a":                                        false,
	} {
		if got := AlwaysConfirm(cmd); got != want {
			t.Errorf("AlwaysConfirm(%q) = %v, want %v", cmd, got, want)
		}
	}
}
