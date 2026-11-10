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
)

const (
	rpmLong  = "--query --all --info --list --queryformat --qf --last --changelog --requires --provides --whatprovides --whatrequires --file --package --scripts --docfiles --configfiles --state --conflicts --obsoletes --recommends --suggests"
	dpkgRead = "-l -L -s -S -p -C --list --listfiles --status --search --print-avail --audit --get-selections --print-architecture --print-foreign-architectures --no-pager --version"
)

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
	case "loginctl":
		return safe(sub == "" || has("list-sessions list-users list-seats show-session show-user show-seat session-status user-status seat-status", sub))
	case "hostnamectl":
		return safe(sub == "" || sub == "status")
	case "systemctl":
		return safe(sub == "" || has("status list-units list-unit-files list-timers list-sockets list-dependencies list-jobs is-active is-enabled is-failed is-system-running show cat get-default", sub))
	case "systemd-analyze":
		return safe(sub == "" || has("time blame critical-chain", sub))
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
		for _, a := range args[1:] {
			if has("--sync --remove --upgrade --database --deptest --refresh --sysupgrade --noconfirm --overwrite --asdeps --asexplicit", a) {
				return Dangerous, true
			}
			if shortFlags.MatchString(a) && strings.ToLower(a) != a {
				return Dangerous, true // another operation (-S, -R, -U, -D…)
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
