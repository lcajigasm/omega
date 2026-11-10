package safety

import (
	"testing"
)

// at pins the working directory and XDG_CONFIG_HOME the policy sees.
func at(t *testing.T, wd, xdg string) {
	t.Helper()
	oldWd, oldXDG := getwd, configHome
	getwd = func() (string, error) { return wd, nil }
	configHome = func() string { return xdg }
	t.Cleanup(func() { getwd, configHome = oldWd, oldXDG })
}

// Nothing may print omega's key file, a keyring or the Keychain, whatever
// the command is wrapped in; and a tool that can change the system is only
// read-only for the subcommands and flags that are.
func TestSecretsAndQueriesThroughEveryDoor(t *testing.T) {
	at(t, "/work/project", "")
	cases := []struct {
		cmd  string
		want Risk
	}{
		// 1. omega's config folder: the key file must be unreachable.
		{"cat ~/.config/omega/keys", Blocked},
		{"cat ~/.config/omega/key*", Blocked},
		{"cat ~/.config/omega/*", Blocked},
		{"head ~/.config/omega/k?ys", Blocked},
		{"cat ~/.config/omega/[k]eys", Blocked},
		{"cat ~/.config/omega/{keys,x}", Blocked},
		{"cat ~/.config/omega/ke{y,}s", Blocked},
		{"cat ~/.config/*/keys", Blocked},
		{"cat ~/.config/om*/k*", Blocked},
		{"cat ~/.config/{omega,gh}/keys", Blocked},
		{"grep -r . ~/.config/omega", Blocked},
		{"grep -R sk- ~/.config/omega/", Blocked},
		{"rg sk- ~/.config/omega", Blocked},
		{"cat ~/.config/omega/'keys'", Blocked},
		{`cat ~/.config/omega/"keys"`, Blocked},
		{`cat ~/.config/omega/ke\ys`, Blocked},
		{`cat ~/.config/omega/k""eys`, Blocked},
		{"cd ~/.config/omega && cat keys", Blocked},
		{"cd ~/.config/omega; cat *", Blocked},
		{"pushd ~/.config/omega", Blocked},
		{"cd ~/.config && cat omega/keys", Blocked},
		{"cd ~/.config && cat omega/k*", Blocked},
		{"cd ~ && cat .config/omega/keys", Blocked},
		{"cd ~/.config && cat omega/*", Blocked},
		{"d=~/.config/omega; cat $d/keys", Blocked},
		{"cat $HOME/.config/omega/keys", Blocked},
		{"cat ${HOME}/.config/omega/keys", Blocked},
		{"cat $XDG_CONFIG_HOME/omega/keys", Blocked},
		{"cat ${XDG_CONFIG_HOME}/omega/k*", Blocked},
		{"cat /home/ana/.config/omega/keys", Blocked},
		{"cat /Users/ana/.config/omega/keys", Blocked},
		{"cat ~/.config/omega/../omega/keys", Blocked},
		{"cp ~/.config/omega/keys /tmp/k", Blocked},
		{"ln -s ~/.config/omega/keys /tmp/k", Blocked},
		{"tar czf /tmp/o.tgz ~/.config/omega", Blocked},
		{"find ~/.config/omega -type f", Blocked},
		{"ls ~/.config/omega | xargs -I{} cat {}", Blocked},
		{"base64 < ~/.config/omega/keys", Blocked},
		{"cat ~/.config/omega/keys.bak", Blocked},
		{"grep -r sk- ~/.config", Blocked},
		{"grep -r sk- ~", Blocked},
		{"grep -rn API $HOME", Blocked},
		{"grep -r . ~/.ssh", Blocked},
		{"cd ~/.ssh && cat mykey", Blocked},
		{"cat /proc/1234/environ", Blocked},
		// What omega's own tools and the user's questions need stays readable.
		{"cat ~/.config/omega/config.toml", Safe},
		{"cat ~/.config/omega/notes.md", Safe},
		{"head -20 $XDG_CONFIG_HOME/omega/mac.toml", Safe},
		{"ls -la ~/.config/omega", Safe},
		{"du -sh ~/.config/omega", Safe},
		{"ls ~/.config", Safe},
		{"grep -rn TODO src", Safe},
		{"grep key ~/.config/omega/config.toml", Safe},
		{"cat omega/main.go", Safe},
		{"echo keys", Safe},
		{"ls ~/.ssh", Safe},

		// 2. Wrappers, shells and substitutions do not hide a secret.
		{"secret-tool lookup service omega-cli account anthropic", Blocked},
		{"env secret-tool lookup service omega-cli account anthropic", Blocked},
		{"env security find-generic-password -s omega-cli -a anthropic -w", Blocked},
		{"env -i HOME=/x security find-generic-password -s omega-cli -w", Blocked},
		{"command secret-tool lookup service omega-cli account openai", Blocked},
		{"exec secret-tool search --all service omega-cli", Blocked},
		{"nice -n 5 secret-tool lookup service omega-cli account openai", Blocked},
		{"nohup security find-generic-password -s omega-cli -w", Blocked},
		{"time secret-tool lookup service omega-cli account openai", Blocked},
		{"echo anthropic | xargs secret-tool lookup service omega-cli account", Blocked},
		{"sudo -u ana secret-tool lookup service omega-cli account openai", Blocked},
		{"sh -c 'secret-tool lookup service omega-cli account anthropic'", Blocked},
		{`bash -c "security find-generic-password -s omega-cli -a anthropic -w"`, Blocked},
		{"echo $(secret-tool lookup service omega-cli account anthropic)", Blocked},
		{"echo `security find-generic-password -s omega-cli -w`", Blocked},
		{"/usr/bin/secret-tool lookup service omega-cli account anthropic", Blocked},
		{"env cat ~/.ssh/id_ed25519", Blocked},
		{"sh -c 'cat ~/.ssh/id_ed25519'", Blocked},
		{"echo $(cat ~/.ssh/id_rsa)", Blocked},
		{"xargs cat < ~/.aws/credentials", Blocked},
		{"bash -c 'cat ~/.config/omega/keys'", Blocked},
		{"echo $(cat ~/.config/omega/keys)", Blocked},
		{"env cat ~/.local/share/keyrings/login.keyring", Blocked},
		{"sh -c 'rm -rf /'", Blocked},
		{"env rm -rf ~", Blocked},
		{"echo $(rm -rf $HOME)", Blocked},
		// env runs what follows it; alone it is harmless.
		{"env", Safe},
		{"env LC_ALL=C ls", Safe},
		{"env brew uninstall node", Dangerous},
		{"env FOO=1 rm -rf /tmp/x", Dangerous},
		{"env sudo apt-get update", Dangerous},
		{"secret-tool store --label=x service omega-cli account openai", Dangerous},
		{"security find-generic-password -s omega-cli", Dangerous},

		// 3. Tools that also change the system: read-only subcommands only.
		{"loginctl", Safe},
		{"loginctl list-sessions", Safe},
		{"loginctl show-user ana", Safe},
		{"loginctl session-status", Safe},
		{"loginctl terminate-user $USER", Dangerous},
		{"loginctl kill-session 3", Dangerous},
		{"loginctl lock-sessions", Dangerous},
		{"loginctl enable-linger ana", Dangerous},
		{"hostnamectl", Safe},
		{"hostnamectl status", Safe},
		{"hostnamectl set-hostname x", Dangerous},
		{"hostnamectl hostname x", Dangerous},
		{"dnf history", Safe},
		{"dnf history list", Safe},
		{"dnf history info 12", Safe},
		{"dnf history undo 12", Dangerous},
		{"dnf history rollback 3", Dangerous},
		{"dnf list --installed", Safe},
		{"rpm -q bash", Safe},
		{"rpm -qa", Safe},
		{"rpm -qi bash", Safe},
		{"rpm -qa --last", Safe},
		{"rpm -q --queryformat '%{NAME}' bash", Safe},
		{"rpm -q --erase bash", Dangerous},
		{"rpm -q -e bash", Dangerous},
		{"rpm -qe bash", Dangerous},
		{"rpm -e bash", Dangerous},
		{"rpm -q --nodeps --erase bash", Dangerous},
		{"rpm --rebuilddb", Dangerous},
		{"dpkg -l", Safe},
		{"dpkg -L bash", Safe},
		{"dpkg -s bash", Safe},
		{"dpkg --get-selections", Safe},
		{"dpkg -l --purge foo", Dangerous},
		{"dpkg --purge -l foo", Dangerous},
		{"dpkg -i foo.deb", Dangerous},
		{"dpkg --configure -a", Dangerous},
		{"dpkg-query -W", Safe},
		{"pacman -Q", Safe},
		{"pacman -Qu", Safe},
		{"pacman -Qdtq", Safe},
		{"pacman -Si bash", Safe},
		{"pacman -Ss ripgrep", Safe},
		{"pacman -Qu; test $? -le 1", Safe},
		{"pacman -Q -S bash", Dangerous},
		{"pacman -Syu", Dangerous},
		{"pacman -Sy", Dangerous},
		{"pacman -Rns foo", Dangerous},
		{"pacman -Q --remove foo", Dangerous},
		{"paccache -d", Safe},
		{"paccache --dryrun -k2", Safe},
		{"paccache -dk1", Safe},
		{"paccache -d -r", Dangerous},
		{"paccache -rk2", Dangerous},
		{"apt-get -s upgrade", Safe},
		{"apt-get --simulate dist-upgrade", Safe},
		{"apt-get -s -o APT::Get::Simulate=false upgrade", Dangerous},
		{"apt-get upgrade", Dangerous},
		{"apt-cache policy bash", Safe},
		{"apt-mark showhold", Safe},
		{"apt-mark hold bash", Dangerous},
		{"systemctl --failed --no-pager --no-legend", Safe},
		{"systemctl --user --failed", Safe},
		{"systemctl status docker", Safe},
		{"systemctl list-timers --all", Safe},
		{"systemctl is-active docker", Safe},
		{"systemctl --failed stop docker", Dangerous},
		{"systemctl stop docker", Dangerous},
		{"systemctl --user disable foo", Dangerous},
		{"systemctl status docker; systemctl restart docker", Dangerous},
		{"systemctl daemon-reload", Dangerous},
		{"systemd-analyze blame", Safe},
		{"systemd-analyze set-log-level debug", Dangerous},
		{"journalctl -b -p err --no-pager", Safe},
		{"journalctl --rotate", Dangerous},
		{"journalctl --vacuum-time=1d", Dangerous},
		{"zypper list-updates", Safe},
		{"zypper remove foo", Dangerous},
		{"flatpak list", Safe},
		{"flatpak uninstall foo", Dangerous},
		{"snap list --all", Safe},
		{"snap refresh --list", Safe},
		{"snap remove foo", Dangerous},
		{"coredumpctl list", Safe},
		{"coredumpctl debug", Dangerous},
		{"fwupdmgr get-updates", Safe},
		{"fwupdmgr update", Dangerous},
		{"ps aux", Safe},
		{"ps -ef", Safe},
		{"ps eww", Dangerous},
		{"ps auxe", Dangerous},
		{"ps -E", Dangerous},
	}
	for _, c := range cases {
		if got := Classify(c.cmd); got != c.want {
			t.Errorf("Classify(%q) = %s, want %s", c.cmd, got, c.want)
		}
	}
}

// Typed inside omega's folder, or with XDG_CONFIG_HOME pointing elsewhere,
// the key file is the same file.
func TestKeyFileByRelativePathAndXDG(t *testing.T) {
	at(t, "/home/ana/.config/omega", "")
	for _, p := range []string{"keys", "./keys", "k*", "../omega/keys", "keys.bak"} {
		if !SecretFile(p) {
			t.Errorf("inside omega's folder, SecretFile(%q) must be true", p)
		}
	}
	for _, p := range []string{"config.toml", "./notes.md", "mac.toml"} {
		if SecretFile(p) {
			t.Errorf("SecretFile(%q) must stay readable", p)
		}
	}
	for cmd, want := range map[string]Risk{"cat keys": Blocked, "cat ./keys": Blocked, "cat *": Blocked, "cat config.toml": Safe, "grep -r sk- .": Blocked} {
		if got := Classify(cmd); got != want {
			t.Errorf("in omega's folder, Classify(%q) = %s, want %s", cmd, got, want)
		}
	}

	at(t, "/home/ana", "")
	if SecretFile("keys") || SecretFile("./keys") {
		t.Error("a file called keys somewhere else is not omega's")
	}
	if !SecretFile(".config/omega/keys") || Classify("cat .config/omega/keys") != Blocked {
		t.Error("a relative path from the home directory reaches the key file")
	}
	if Classify("rg sk-") != Blocked || Classify("grep -r sk- .") != Blocked {
		t.Error("a recursive search of the home directory reads the keys")
	}

	at(t, "/work", "/data/cfg")
	for cmd, want := range map[string]Risk{
		"cat /data/cfg/omega/keys": Blocked, "cat /data/cfg/omega/*": Blocked, "cat /data/cfg/*/keys": Blocked,
		"grep -r . /data/cfg/omega": Blocked, "grep -r . /data/cfg": Blocked, "cd /data/cfg/omega && cat keys": Blocked,
		"cat /data/cfg/omega/config.toml": Safe, "ls /data/cfg/omega": Safe,
	} {
		if got := Classify(cmd); got != want {
			t.Errorf("with XDG_CONFIG_HOME, Classify(%q) = %s, want %s", cmd, got, want)
		}
	}
	if !SecretFile("/data/cfg/omega/keys") || SecretFile("/data/cfg/omega/notes.md") {
		t.Error("SecretFile must follow XDG_CONFIG_HOME")
	}
}
