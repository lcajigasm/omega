package assist

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The maintenance tools omega drives itself: topgrade and Mole. Their
// settings below come from what the run logs showed going wrong: an image
// pulled on every run that has no registry copy, a sudo check that stopped
// every unattended run, and a cache of 4 GB that Chrome downloads again.

func init() {
	register("topgrade", topgrade)
	register("mole", mole)
}

// ---------- topgrade ----------

// omegaCI is omega's own CI image: built locally, never pushed, so every
// `docker pull` of it from topgrade's containers step fails.
const omegaCI = "omega:ci*"

func (e *env) configDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x
	}
	return e.path(".config")
}

// topgradeFiles are the configuration files topgrade reads: its main file
// and the drop-ins in topgrade.d, which it reads first (so the main file
// wins).
func (e *env) topgradeFiles() (main string, dropins []string) {
	main = filepath.Join(e.configDir(), "topgrade.toml")
	if e.goos == "darwin" && !exists(main) {
		if alt := e.path("Library", "Application Support", "topgrade.toml"); exists(alt) {
			main = alt
		}
	}
	dropins, _ = filepath.Glob(filepath.Join(filepath.Dir(main), "topgrade.d", "*.toml"))
	sort.Strings(dropins)
	return main, dropins
}

func topgrade(e *env) Tool {
	t := Tool{Name: "topgrade", Kind: "cli", About: "Updates everything installed with one command; omega runs it as a step."}
	bin, ok := e.has("topgrade")
	t.Installed = ok
	if !ok {
		return t
	}
	t.Version = cliVersion(bin)
	main, dropins := e.topgradeFiles()
	t.Where = main
	files := append(append([]string{}, dropins...), main)
	dropin := filepath.Join(filepath.Dir(main), "topgrade.d", "omega.toml")

	// ignored_containers: the main file wins over the drop-ins, so the
	// list that counts is the last one set.
	var ignored []string
	for _, f := range files {
		if v, isList := tomlGet(f, "containers", "ignored_containers").([]any); isList {
			ignored = ignored[:0]
			for _, x := range v {
				if s, isStr := x.(string); isStr {
					ignored = append(ignored, s)
				}
			}
		}
	}
	has := false
	for _, s := range ignored {
		has = has || s == omegaCI || s == "omega:ci"
	}
	ci := Check{ID: "topgrade.omega-ci", Title: "Leave omega's CI image out of topgrade's pulls", Kind: KindAction, File: dropin,
		Why:     "omega's CI image (omega:ci) is built on this machine and has no registry copy: topgrade's containers step tries to pull it on every run and reports a failure.",
		Current: map[bool]string{true: "ignored", false: "pulled on every run"}[has], Recommended: "ignore", OK: has}
	ci.set = func(string) (string, error) {
		// Written where the list that counts lives, keeping what is there.
		target := dropin
		for _, f := range files {
			if tomlGet(f, "containers", "ignored_containers") != nil {
				target = f
			}
		}
		list := append(append([]string{}, ignored...), omegaCI)
		quoted := make([]string, len(list))
		for i, s := range list {
			quoted[i] = strconv.Quote(s)
		}
		return tomlSet("topgrade", target, "containers", [][2]string{{"ignored_containers", "[" + strings.Join(quoted, ", ") + "]"}},
			"topgrade leaves "+omegaCI+" out of its pulls")
	}
	t.Checks = append(t.Checks, ci)

	// pre_sudo and sudo_loop run `sudo -v` before anything else, and the
	// whole run stops when nobody can answer.
	var where []string
	for _, f := range files {
		for _, k := range []string{"pre_sudo", "sudo_loop"} {
			if v, isBool := tomlGet(f, "misc", k).(bool); isBool && v {
				where = append(where, k+" in "+f)
			}
		}
	}
	sudo := Check{ID: "topgrade.pre-sudo", Title: "Do not ask for sudo before every topgrade run", Kind: KindSetting, File: main,
		Why: "pre_sudo and sudo_loop make topgrade run sudo -v before any step, and stop the whole run when sudo cannot be answered " +
			"(a scheduled or unattended run). Under omega a step that needs sudo asks omega for it when it runs.",
		Current: "off", Recommended: "off", OK: len(where) == 0}
	if len(where) > 0 {
		sudo.Current = "on (" + strings.Join(where, "; ") + ")"
	}
	sudo.set = func(string) (string, error) {
		var done []string
		for _, f := range files {
			var vals [][2]string
			for _, k := range []string{"pre_sudo", "sudo_loop"} {
				if v, isBool := tomlGet(f, "misc", k).(bool); isBool && v {
					vals = append(vals, [2]string{k, "false"})
				}
			}
			if len(vals) == 0 {
				continue
			}
			msg, err := tomlSet("topgrade", f, "misc", vals, "topgrade no longer asks for sudo up front")
			if err != nil {
				return "", err
			}
			done = append(done, msg)
		}
		if len(done) == 0 {
			return "topgrade does not ask for sudo up front: nothing to change", nil
		}
		return strings.Join(done, "\n"), nil
	}
	t.Checks = append(t.Checks, sudo)
	return t
}

// ---------- Mole ----------

// chromeModel is the on-device AI model Chrome keeps in its Application
// Support folder: about 4 GB that Mole's clean removed and Chrome then
// downloaded again.
const chromeModel = "$HOME/Library/Application Support/Google/Chrome/OptGuideOnDeviceModel/*"

// moleDefaults reads Mole's own default whitelist from the copy that is
// installed (lib/core/base.sh). A whitelist file replaces those defaults,
// so a file omega writes has to start with them.
var moleDefaults = func(bin string) ([]string, error) {
	real, err := filepath.EvalSymlinks(bin)
	if err != nil {
		return nil, err
	}
	var base string
	for _, c := range []string{
		filepath.Join(filepath.Dir(real), "lib", "core", "base.sh"),
		filepath.Join(filepath.Dir(real), "..", "libexec", "lib", "core", "base.sh"),
		filepath.Join(filepath.Dir(real), "..", "lib", "core", "base.sh"),
	} {
		if exists(c) {
			base = c
			break
		}
	}
	if base == "" {
		return nil, fmt.Errorf("Mole's default whitelist (lib/core/base.sh) was not found next to %s", real)
	}
	f, err := os.Open(base)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case strings.Contains(l, "DEFAULT_WHITELIST_PATTERNS=("):
			in = true
		case in && l == ")":
			if len(out) == 0 {
				return nil, fmt.Errorf("Mole's default whitelist in %s is empty", base)
			}
			return out, nil
		case in && l != "" && !strings.HasPrefix(l, "#"):
			v := strings.Trim(l, `"'`)
			// The sentinel is a variable Mole always adds itself.
			if !strings.HasPrefix(v, "$FINDER") {
				out = append(out, v)
			}
		}
	}
	return nil, fmt.Errorf("Mole's default whitelist was not found in %s", base)
}

func mole(e *env) Tool {
	t := Tool{Name: "Mole", Kind: "cli", About: "Cleans and optimises the Mac (mo); omega runs it as a step."}
	bin, ok := e.has("mo")
	t.Installed = ok && e.goos == "darwin"
	if !t.Installed {
		return t
	}
	t.Version = cliVersion(bin)
	dir := filepath.Join(e.configDir(), "mole")
	t.Where = dir
	list := filepath.Join(dir, "whitelist")
	model := e.path("Library", "Application Support", "Google", "Chrome", "OptGuideOnDeviceModel")
	if !isDir(model) {
		return t
	}
	data, _ := os.ReadFile(list)
	kept := strings.Contains(string(data), "OptGuideOnDeviceModel")
	n := size(model)
	c := Check{ID: "mole.chrome-model", Title: "Keep Chrome's on-device AI model when Mole cleans", Kind: KindAction, Disk: true, File: list,
		Size: n, Current: map[bool]string{true: "kept", false: "deleted by mo clean, then downloaded again by Chrome"}[kept],
		Why: "Chrome keeps its on-device model in Application Support; mo clean deletes it as a cache and Chrome downloads it again, " +
			"so the space is never really freed. omega adds it to Mole's whitelist (starting the file with Mole's own defaults, " +
			"which a whitelist file replaces).",
		Recommended: "keep", OK: kept}
	c.set = func(string) (string, error) {
		var lines []string
		if len(data) == 0 {
			defaults, err := moleDefaults(bin)
			if err != nil {
				return "", fmt.Errorf("%v; the whitelist was not written, because a new file would drop Mole's defaults", err)
			}
			lines = append([]string{"# Mole's default whitelist, copied by omega (a whitelist file replaces it)"}, defaults...)
		} else {
			lines = []string{strings.TrimRight(string(data), "\n")}
		}
		lines = append(lines, "# Chrome's on-device AI model: Chrome downloads it again when it is deleted (omega)", chromeModel)
		return change("mole", list, []byte(strings.Join(lines, "\n")+"\n"), "Mole keeps Chrome's on-device model")
	}
	t.Checks = append(t.Checks, c)
	return t
}
