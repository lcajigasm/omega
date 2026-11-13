package safety

import (
	"regexp"
	"strings"
)

// Tools that both query and change the system are not trusted by prefix:
// "rpm -q --erase" starts like a query. Each one is read-only only when
// every argument is known to be, and dangerous otherwise.

func has(list string, w string) bool { return strings.Contains(" "+list+" ", " "+w+" ") }

// subcommand is the first argument that is not an option.
func subcommand(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

var (
	rpmShort    = regexp.MustCompile(`^-q[ailcdfpRsv]*$|^-[ailcdfpRsv]+$`)
	pacmanQuery = regexp.MustCompile(`^-Q[a-z]*$|^-S[si]+$|^-F[lxo]+$`)
	shortFlags  = regexp.MustCompile(`^-[A-Za-z0-9]+$`)
	psEnvBSD    = regexp.MustCompile(`^[A-Za-z]*e[A-Za-z]*$`)
	listOrGet   = regexp.MustCompile(`^-(list|get)\w*$`)
	ifaceName   = regexp.MustCompile(`^[a-z]+[0-9]*$`)
	lsofRepeats = regexp.MustCompile(`^[-+][A-Za-z]*r`)
	netstatFlag = regexp.MustCompile(`^-[A-Za-z46]+$`)
	xattrReads  = regexp.MustCompile(`^-[lprsvx]+$`)
	// What makes an awk program more than a filter: it runs a command,
	// reads from one, or prints into a file or a pipe.
	awkActs = regexp.MustCompile(`system|getline|close|fflush|printf?[^;}]*[>|]`)
)

const (
	rpmLong  = "--query --all --info --list --queryformat --qf --last --changelog --requires --provides --whatprovides --whatrequires --file --package --scripts --docfiles --configfiles --state --conflicts --obsoletes --recommends --suggests"
	dpkgRead = "-l -L -s -S -p -C --list --listfiles --status --search --print-avail --audit --get-selections --print-architecture --print-foreign-architectures --no-pager --version"
	// Options of systemctl and its relatives that stand alone, and the
	// ones written --name=value that only filter or format.
	ctlAlone = "--no-pager --user --system --all -a --full -l --failed --no-legend --plain --quiet -q --no-ask-password --value --reverse --after --before --recursive --show-types --with-dependencies"
	ctlValue = "--type --state --property --output --lines"
	// And the ones that take a value as the next word. Before the
	// subcommand such a value would be read as the subcommand.
	ctlNext     = "-t -p -o -n"
	pkgutilRead = "--pkgs --packages --pkg-info --pkg-info-plist --files --file-info --file-info-plist --only-files --only-dirs --regexp --volume --pkg-groups --groups --group-pkgs --check-signature"
	gitTagList  = "-l --list -n -i --ignore-case --contains --no-contains --merged --no-merged --points-at --sort --format --column --no-column --color"
)

// optionOverrides reports an option that changes how a package manager
// works instead of what it shows: a configuration value (-o, --setopt), a
// configuration file, another root, or "yes to everything".
func optionOverrides(args []string) bool {
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if (strings.HasPrefix(a, "-o") && !strings.HasPrefix(a, "--")) || strings.HasPrefix(a, "--setopt") ||
			has("--option -c --config-file --config --installroot --root -R -y --yes --assume-yes --assumeyes --noconfirm", name) {
			return true
		}
	}
	return false
}

// queryTool classifies a command of one of those tools. ok is false for
// any other program, which the prefix lists decide.
func queryTool(p string) (risk Risk, ok bool) {
	f := strings.Fields(p)
	if len(f) == 0 {
		return Safe, false
	}
	args := f[1:]
	safe := func(yes bool) (Risk, bool) {
		if yes {
			return Safe, true
		}
		return Dangerous, true
	}
	sub := subcommand(args)
	switch f[0] {
	case "loginctl", "hostnamectl", "systemctl", "systemd-analyze":
		// An option that takes a value hides the real subcommand:
		// "systemctl -p status poweroff" powers the machine off. And one
		// that names another machine (-H, --host, -M) is not a look at
		// this one, wherever it is written. Only options known to be
		// harmless pass.
		seen := false
		for i := 0; i < len(args); i++ {
			a := args[i]
			name, _, hasValue := strings.Cut(a, "=")
			switch {
			case !strings.HasPrefix(a, "-"):
				seen = true
			case hasValue && has(ctlValue, name), !hasValue && has(ctlAlone, a):
			case seen && has(ctlNext, a):
				i++
			default:
				return Dangerous, true
			}
		}
	case "apt", "apt-cache", "apt-mark", "dnf", "yum", "zypper":
		if optionOverrides(args) {
			return Dangerous, true
		}
	}
	switch f[0] {
	case "loginctl":
		return safe(sub == "" || has("list-sessions list-users list-seats show-session show-user show-seat session-status user-status seat-status", sub))
	case "hostnamectl":
		return safe(sub == "" || sub == "status")
	case "systemctl":
		return safe(sub == "" || has("status list-units list-unit-files list-timers list-sockets list-dependencies list-jobs is-active is-enabled is-failed is-system-running show cat get-default", sub))
	case "systemd-analyze":
		return safe(sub == "" || has("time blame critical-chain", sub))
	case "cd":
		// Only for the rest of the line: each command starts in a new shell.
		n := 0
		for _, a := range args {
			if a != "-" && !strings.HasPrefix(a, "-") {
				n++
			}
		}
		return safe(n <= 1)
	case "[", "[[":
		return safe(len(args) > 0 && args[len(args)-1] == strings.Replace(f[0], "[", "]", -1))
	case "git":
		if len(args) == 0 {
			return Safe, false
		}
		switch rest := args[1:]; args[0] {
		case "remote":
			// "git remote -v add name url" adds one: nothing but -v may follow,
			// or one of the two subcommands that only show.
			if len(rest) > 0 && (rest[0] == "show" || rest[0] == "get-url") {
				for _, a := range rest[1:] {
					if strings.HasPrefix(a, "-") && !has("-n --all --push", a) {
						return Dangerous, true
					}
				}
				return Safe, true
			}
			return safe(allIn("-v --verbose", rest))
		case "tag":
			lists := false
			for _, a := range rest {
				name, _, _ := strings.Cut(a, "=")
				switch {
				case strings.HasPrefix(a, "-n") && shortFlags.MatchString(a):
				case strings.HasPrefix(a, "-"):
					if !has(gitTagList, name) {
						return Dangerous, true
					}
					lists = lists || has("-l --list --contains --no-contains --merged --no-merged --points-at", name)
				case !lists:
					return Dangerous, true // a name alone creates a tag
				}
			}
			return Safe, true
		}
		return Safe, false
	case "softwareupdate":
		// -l lists; -i, -d, -R and the rest download, install and restart.
		lists := false
		for _, a := range args {
			switch {
			case a == "-l" || a == "--list" || a == "--history":
				lists = true
			case !has("-a --all --no-scan --verbose --include-config-data", a):
				return Dangerous, true
			}
		}
		return safe(lists)
	case "mdutil":
		// -s reports the state of the index; -E erases it, -i and -d switch it.
		status := false
		for _, a := range args {
			if strings.HasPrefix(a, "-") {
				if strings.Trim(a, "-sav") != "" {
					return Dangerous, true
				}
				status = status || strings.Contains(a, "s")
			}
		}
		return safe(status)
	case "xcode-select":
		return safe(len(args) == 1 && has("-p --print-path -v --version", args[0]))
	case "spctl":
		return safe(len(args) == 1 && args[0] == "--status")
	case "scutil":
		return safe((len(args) == 2 && args[0] == "--get") || (len(args) == 1 && has("--dns --proxy --nwi", args[0])))
	case "networksetup":
		if len(args) == 0 || !listOrGet.MatchString(args[0]) {
			return Dangerous, true
		}
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				return Dangerous, true
			}
		}
		return Safe, true
	case "plutil":
		if len(args) < 2 || !has("-p -lint", args[0]) {
			return Dangerous, true
		}
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				return Dangerous, true
			}
		}
		return Safe, true
	case "pkgutil":
		for _, a := range args {
			name, _, _ := strings.Cut(a, "=")
			if strings.HasPrefix(a, "-") && !has(pkgutilRead, name) {
				return Dangerous, true
			}
		}
		return safe(len(args) > 0)
	case "xattr":
		// -w writes an attribute, -d deletes one, -c clears them all.
		for _, a := range args {
			if strings.HasPrefix(a, "-") && !xattrReads.MatchString(a) {
				return Dangerous, true
			}
		}
		return safe(len(args) > 0)
	case "ifconfig":
		// With more than an interface name it configures the interface.
		return safe(len(args) == 0 || (len(args) == 1 && (has("-a -l -u", args[0]) || ifaceName.MatchString(args[0]))))
	case "netstat":
		// -w and -c repeat for ever, and so does an interval after the options.
		for i := 0; i < len(args); i++ {
			a := args[i]
			switch {
			case (a == "-p" || a == "-f") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-"):
				i++
			case !netstatFlag.MatchString(a) || strings.ContainsAny(a, "wcI"):
				return Dangerous, true
			}
		}
		return Safe, true
	case "lsof":
		for _, a := range args {
			if lsofRepeats.MatchString(a) {
				return Dangerous, true // -r repeats until it is killed
			}
		}
		return Safe, true
	case "man":
		// -P and -H run a program (the pager, a browser), -C reads another
		// configuration. A name, a section and the lookup options are left.
		for _, a := range args {
			if strings.HasPrefix(a, "-") && !has("-k -f -a -w", a) {
				return Dangerous, true
			}
		}
		return safe(len(args) > 0)
	case "awk", "gawk", "mawk", "nawk":
		for _, a := range args {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "-F") && !strings.HasPrefix(a, "-v") {
				return Dangerous, true // -f runs a script nobody has read, -i edits in place
			}
			if awkActs.MatchString(a) {
				return Dangerous, true
			}
		}
		return safe(len(args) > 0)
	case "nvram":
		return safe(len(args) > 0 && allIn(nvramReads, args))
	case "fdesetup":
		return safe(len(args) == 1 && has(fdesetupReads, args[0]))
	case "dnf":
		if sub != "history" {
			return Safe, false
		}
		for i, a := range args {
			if a == "history" {
				next := subcommand(args[i+1:])
				return safe(next == "" || next == "list" || next == "info")
			}
		}
	case "rpm":
		if len(args) == 0 || !(strings.HasPrefix(args[0], "-q") || args[0] == "--query") {
			return Dangerous, true
		}
		for _, a := range args {
			switch {
			case strings.HasPrefix(a, "--"):
				name, _, _ := strings.Cut(a, "=")
				if !has(rpmLong, name) {
					return Dangerous, true
				}
			case strings.HasPrefix(a, "-"):
				if !rpmShort.MatchString(a) {
					return Dangerous, true
				}
			}
		}
		return Safe, true
	case "dpkg":
		read := false
		for _, a := range args {
			if !strings.HasPrefix(a, "-") {
				continue
			}
			if !has(dpkgRead, a) {
				return Dangerous, true
			}
			read = true
		}
		return safe(read)
	case "pacman":
		if len(args) == 0 || !pacmanQuery.MatchString(args[0]) {
			return Dangerous, true
		}
		// Under -S and -F the letters mean something else than under -Q:
		// -y refreshes the databases, -c empties the cache, -u upgrades.
		query := strings.HasPrefix(args[0], "-Q")
		for _, a := range args[1:] {
			if has("--sync --remove --upgrade --database --deptest --refresh --sysupgrade --noconfirm --overwrite --asdeps --asexplicit --clean --downloadonly", a) {
				return Dangerous, true
			}
			if shortFlags.MatchString(a) && strings.ToLower(a) != a {
				return Dangerous, true // another operation (-S, -R, -U, -D…)
			}
			if !query && shortFlags.MatchString(a) && strings.Trim(a, "-siqlg") != "" {
				return Dangerous, true
			}
			if !query && strings.HasPrefix(a, "--") && !has("--info --search --list --quiet --groups --color --regex", a) {
				return Dangerous, true
			}
		}
		return Safe, true
	case "paccache":
		dry := false
		for _, a := range args {
			switch {
			case a == "--dryrun":
				dry = true
			case a == "--remove" || a == "--move":
				return Dangerous, true
			case shortFlags.MatchString(a):
				if strings.ContainsAny(a, "rm") {
					return Dangerous, true
				}
				dry = dry || strings.Contains(a, "d")
			}
		}
		return safe(dry)
	case "apt-get":
		simulate := false
		for _, a := range args {
			switch {
			case has("-s --simulate --just-print --dry-run --recon --no-act", a):
				simulate = true
			case strings.HasPrefix(a, "-o") || strings.HasPrefix(a, "--option") || strings.HasPrefix(a, "-c") || strings.HasPrefix(a, "--config-file"):
				return Dangerous, true // an option can switch the simulation off
			case strings.HasPrefix(a, "--no-") || (strings.HasPrefix(a, "-") && strings.Contains(a, "=")):
				return Dangerous, true // and so can --no-simulate and -s=no
			}
		}
		return safe(simulate)
	case "ps":
		// "ps e" (BSD style) and -E print each process's environment, which
		// can hold an API key.
		for _, a := range args {
			if a == "-E" || (!strings.HasPrefix(a, "-") && psEnvBSD.MatchString(a)) || (shortFlags.MatchString(a) && strings.Contains(a, "E")) {
				return Dangerous, true
			}
		}
	}
	return Safe, false
}
