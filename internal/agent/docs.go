package agent

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/lcajigasm/omega/internal/runner"
)

// docSource is an official reference of an operating system: who publishes
// it, what it covers and where it is. omega ships the addresses, never the
// advice: the model reads the page with fetch_url when it needs it, so what
// it applies is what the vendor documents today.
type docSource struct {
	Name   string
	OS     string // "darwin", "linux" or "windows"
	Distro string // for Linux: an ID or ID_LIKE of /etc/os-release; "" for every distribution
	Covers string
	URLs   []string
}

var platformDocs = []docSource{
	{OS: "darwin", Name: "macOS User Guide (Apple)", Covers: "settings, storage, login items, Time Machine, troubleshooting",
		URLs: []string{"https://support.apple.com/guide/mac-help/welcome/mac"}},
	{OS: "darwin", Name: "Apple Platform Deployment", Covers: "software updates, launchd and background items, managed settings, FileVault",
		URLs: []string{"https://support.apple.com/guide/deployment/welcome/web"}},
	{OS: "darwin", Name: "Apple Platform Security", Covers: "Gatekeeper, System Integrity Protection, privacy permissions (TCC), the keychain",
		URLs: []string{"https://support.apple.com/guide/security/welcome/web"}},
	{OS: "darwin", Name: "Terminal and Disk Utility guides (Apple)", Covers: "the shell on a Mac, disks, volumes, APFS snapshots, First Aid",
		URLs: []string{"https://support.apple.com/guide/terminal/welcome/mac", "https://support.apple.com/guide/disk-utility/welcome/mac"}},
	{OS: "darwin", Name: "macOS release notes and security releases (Apple)", Covers: "what each macOS version changed, known issues, security fixes",
		URLs: []string{"https://developer.apple.com/documentation/macos-release-notes", "https://support.apple.com/en-us/100100"}},

	{OS: "windows", Name: "Windows commands reference (Microsoft Learn)", Covers: "DISM, sfc, chkdsk, cleanmgr, powercfg and every built-in command",
		URLs: []string{"https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/windows-commands"}},
	{OS: "windows", Name: "Repair a Windows image (Microsoft Learn)", Covers: "the component store: DISM /Online /Cleanup-Image, in the order Microsoft documents",
		URLs: []string{"https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/repair-a-windows-image"}},
	{OS: "windows", Name: "Windows client troubleshooting (Microsoft Learn)", Covers: "Windows Update errors, performance, startup, networking",
		URLs: []string{"https://learn.microsoft.com/en-us/troubleshoot/windows-client/welcome-windows-client"}},
	{OS: "windows", Name: "Windows release health (Microsoft Learn)", Covers: "known issues of each Windows version and update, and their status",
		URLs: []string{"https://learn.microsoft.com/en-us/windows/release-health/"}},
	{OS: "windows", Name: "PowerShell documentation (Microsoft Learn)", Covers: "cmdlets, profiles, execution policy, modules",
		URLs: []string{"https://learn.microsoft.com/en-us/powershell/scripting/overview"}},

	{OS: "linux", Name: "systemd manual pages (freedesktop.org)", Covers: "units, timers, journald, logind, resolved, tmpfiles",
		URLs: []string{"https://www.freedesktop.org/software/systemd/man/latest/"}},
	{OS: "linux", Name: "Linux man-pages and the kernel's administration guide", Covers: "every command and system call, sysctl, kernel parameters",
		URLs: []string{"https://man7.org/linux/man-pages/", "https://docs.kernel.org/admin-guide/"}},
	{OS: "linux", Distro: "debian", Name: "Debian Reference, the Administrator's Handbook and the wiki", Covers: "apt, dpkg, system upkeep, release upgrades",
		URLs: []string{"https://www.debian.org/doc/manuals/debian-reference/", "https://www.debian.org/doc/manuals/debian-handbook/", "https://wiki.debian.org/", "https://manpages.debian.org/"}},
	{OS: "linux", Distro: "ubuntu", Name: "Ubuntu Server documentation and manual pages", Covers: "apt, snap, unattended upgrades, release upgrades, services",
		URLs: []string{"https://documentation.ubuntu.com/server/", "https://manpages.ubuntu.com/"}},
	{OS: "linux", Distro: "fedora", Name: "Fedora documentation and Quick Docs", Covers: "dnf, upgrades between releases, SELinux, firewalld",
		URLs: []string{"https://docs.fedoraproject.org/en-US/docs/", "https://docs.fedoraproject.org/en-US/quick-docs/"}},
	{OS: "linux", Distro: "arch", Name: "ArchWiki and Arch manual pages", Covers: "system maintenance, pacman, the package cache, .pacnew files, the AUR",
		URLs: []string{"https://wiki.archlinux.org/title/System_maintenance", "https://wiki.archlinux.org/title/General_recommendations", "https://man.archlinux.org/"}},
	{OS: "linux", Distro: "suse", Name: "openSUSE documentation", Covers: "zypper, snapper and Btrfs snapshots, YaST, system upgrades",
		URLs: []string{"https://doc.opensuse.org/"}},
}

// localDocs is how to read the documentation installed on the machine, which
// matches the installed version exactly.
func localDocs(goos string) string {
	switch goos {
	case "windows":
		return "Get-Help <cmdlet> -Full ; <program> /? ; <program> --help"
	case "darwin":
		return "man <command> | col -b ; <command> --help (e.g. man launchctl, man pmset, man tmutil, man softwareupdate, man defaults)"
	}
	return "man <command> | col -b ; <command> --help ; ls /usr/share/doc/<package>"
}

// linuxIDs are the ID and ID_LIKE words of /etc/os-release, lower case:
// "ubuntu debian" for Ubuntu, "fedora" for Fedora, "opensuse-tumbleweed
// opensuse suse" for Tumbleweed.
func linuxIDs(osRelease string) []string {
	var out []string
	for _, line := range strings.Split(osRelease, "\n") {
		line = strings.TrimSpace(line)
		for _, key := range []string{"ID=", "ID_LIKE="} {
			if v, ok := strings.CutPrefix(line, key); ok {
				out = append(out, strings.Fields(strings.ToLower(strings.Trim(v, `"'`)))...)
			}
		}
	}
	return out
}

// platformDocsHere are the references of the system omega runs on: every
// one of the operating system, and on Linux those of the distribution and of
// the ones it derives from.
func platformDocsHere() []docSource {
	var ids []string
	if goos == "linux" {
		ids = linuxIDs(readOSRelease())
	}
	var out []docSource
	for _, d := range platformDocs {
		if d.OS != goos {
			continue
		}
		if d.Distro != "" {
			match := false
			for _, id := range ids {
				match = match || id == d.Distro || strings.HasSuffix(id, d.Distro)
			}
			if !match {
				continue
			}
		}
		out = append(out, d)
	}
	return out
}

// hintsHere are the tool hints that apply to this machine: the tool is
// installed and the hint is for this operating system.
func hintsHere() []hint {
	var out []hint
	for _, h := range hints {
		if (h.Bin == "" || hasBin(h.Bin)) && (h.OS == "" || h.OS == goos) {
			out = append(out, h)
		}
	}
	return out
}

// hasBin is replaced by tests.
var hasBin = runner.Has

// officialHosts are the hosts of every reference omega knows, without the
// code hosts, where anybody publishes.
var officialHosts = func() map[string]bool {
	out := map[string]bool{}
	add := func(urls []string) {
		for _, u := range urls {
			if pu, err := url.Parse(u); err == nil && pu.Host != "" && pu.Host != "github.com" {
				out[strings.TrimPrefix(pu.Host, "www.")] = true
			}
		}
	}
	for _, d := range platformDocs {
		add(d.URLs)
	}
	for _, h := range hints {
		add(h.Docs)
	}
	return out
}()

// officialURL reports whether a link points at one of the official
// documentation sites omega knows.
func officialURL(link string) bool {
	pu, err := url.Parse(link)
	if err != nil || pu.Scheme != "https" {
		return false
	}
	return officialHosts[strings.TrimPrefix(strings.ToLower(pu.Host), "www.")]
}

// toolOfficialDocs answers "where is this documented?": the official
// references of this operating system and of the tools installed here.
func toolOfficialDocs(_ context.Context, a *Agent, in map[string]any) (string, error) {
	topic := strings.ToLower(strings.TrimSpace(getString(in, "topic")))
	matches := func(texts ...string) bool {
		if topic == "" {
			return true
		}
		for _, t := range texts {
			if strings.Contains(strings.ToLower(t), topic) {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	var system []docSource
	for _, d := range platformDocsHere() {
		if matches(d.Name, d.Covers) {
			system = append(system, d)
		}
	}
	var tools []hint
	for _, h := range hintsHere() {
		if matches(h.Tool, h.Bin) {
			tools = append(tools, h)
		}
	}
	if len(system) == 0 && len(tools) == 0 {
		// Nothing by that name: the system's references still apply.
		system = platformDocsHere()
		fmt.Fprintf(&b, "omega has no reference filed under %q. Find the vendor's own documentation with web_search (\"%s official documentation\") "+
			"and prefer the results marked [official]. The references of this system follow.\n\n", topic, topic)
	}
	if len(system) > 0 {
		fmt.Fprintf(&b, "## %s: official documentation\n", osName())
		for _, d := range system {
			fmt.Fprintf(&b, "- %s (%s): %s\n", d.Name, d.Covers, strings.Join(d.URLs, " "))
		}
		fmt.Fprintf(&b, "- installed on this machine, and matching the installed version: %s\n\n", localDocs(goos))
	}
	if len(tools) > 0 {
		b.WriteString("## Tools installed here\n")
		for _, h := range tools {
			fmt.Fprintf(&b, "- %s: %s\n", h.Tool, strings.Join(h.Docs, " "))
		}
		b.WriteString("(config_hints says where each of these keeps its configuration and how to inspect it.)\n\n")
	}
	b.WriteString("How to use them: read the page with fetch_url before a fix, an optimisation or a setting that is not a catalogue step; " +
		"check that it applies to the installed version (the tool's --version, system_info); apply only what the documentation supports, " +
		"and name the source in the option's detail or in your summary. A page that does not open is not a reason to guess: try the " +
		"documentation installed on the machine or web_search.")
	return b.String(), nil
}
