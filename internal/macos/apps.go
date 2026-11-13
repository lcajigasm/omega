package macos

import (
	"debug/macho"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lcajigasm/omega/internal/i18n"
)

// AppInfo is an installed app with what a review needs: how big it is, when
// it was last opened, where it came from and whether it still needs Rosetta.
type AppInfo struct {
	App
	Size     int64     // bytes; 0 when unknown
	LastUsed time.Time // zero when Spotlight does not know
	Cask     string    // Homebrew cask that installed it
	Store    bool      // from the Mac App Store
	Intel    bool      // no Apple silicon slice: runs under Rosetta
	Verdict  string
	Why      string
}

// Reasons an app is marked for review.
const (
	WhyUnused = "unused"
	WhyIntel  = "intel"
)

// UnusedApp is how long without opening an app before omega asks about it.
const UnusedApp = 180 * 24 * time.Hour

// hostARM is whether this Mac has Apple silicon; tests set it.
var hostARM = runtime.GOARCH == "arm64"

// caskrooms are where Homebrew keeps what each cask installed.
var caskrooms = []string{"/opt/homebrew/Caskroom", "/usr/local/Caskroom"}

var (
	caskAppJSON = regexp.MustCompile(`"app":\s*\[\s*"([^"]+\.app)"`)
	caskAppRuby = regexp.MustCompile(`(?m)^\s*app\s+"([^"]+\.app)"`)
)

// caskApps maps an app's file name ("Ghostty.app") to the cask that owns it.
func caskApps() map[string]string {
	out := map[string]string{}
	for _, room := range caskrooms {
		files, _ := filepath.Glob(filepath.Join(room, "*", ".metadata", "*", "*", "Casks", "*"))
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			token := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(f), ".json"), ".rb")
			for _, re := range []*regexp.Regexp{caskAppJSON, caskAppRuby} {
				for _, m := range re.FindAllStringSubmatch(string(data), -1) {
					out[filepath.Base(m[1])] = token
				}
			}
		}
	}
	return out
}

// intelOnly reports whether a Mach-O executable has no arm64 slice.
func intelOnly(exe string) bool {
	if ff, err := macho.OpenFat(exe); err == nil {
		defer ff.Close()
		for _, a := range ff.Arches {
			if a.Cpu == macho.CpuArm64 {
				return false
			}
		}
		return true
	}
	if f, err := macho.Open(exe); err == nil {
		defer f.Close()
		return f.Cpu == macho.CpuAmd64
	}
	return false
}

// diskSize is the space a path takes, in bytes (0 when it cannot be read).
func diskSize(path string) int64 {
	out, err := Output("du", "-sk", path)
	if err != nil && len(out) == 0 {
		return 0
	}
	kb, _ := strconv.ParseInt(strings.Fields(string(out) + " 0")[0], 10, 64)
	return kb * 1024
}

// each runs fn for every index with a few workers: du and mdls are slow
// one by one and independent.
func each(n int, fn func(i int)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			fn(i)
			<-sem
		}(i)
	}
	wg.Wait()
}

// Apps lists the apps the user installed (Apple's own are left out), largest first.
func Apps(p *UserPolicy, installed map[string]App, now time.Time) []AppInfo {
	casks := caskApps()
	var apps []AppInfo
	for _, a := range installed {
		if strings.HasPrefix(a.Path, "/System/") || strings.HasPrefix(a.Bundle, "com.apple.") {
			continue
		}
		apps = append(apps, AppInfo{App: a, Cask: casks[filepath.Base(a.Path)]})
	}
	each(len(apps), func(i int) {
		a := &apps[i]
		a.Size = diskSize(a.Path)
		a.LastUsed = lastUsed(a.Path)
		_, err := os.Stat(filepath.Join(a.Path, "Contents", "_MASReceipt", "receipt"))
		a.Store = err == nil
		if hostARM {
			if exe := asString(readPlist(filepath.Join(a.Path, "Contents", "Info.plist"))["CFBundleExecutable"]); exe != "" {
				a.Intel = intelOnly(filepath.Join(a.Path, "Contents", "MacOS", exe))
			}
		}
	})
	for i := range apps {
		a := &apps[i]
		a.Verdict = Keep
		switch {
		case p != nil && match(p.Apps.Keep, a.Name, a.Bundle):
			a.Why = "the user's rules keep it"
		case !a.LastUsed.IsZero() && now.Sub(a.LastUsed) > UnusedApp:
			a.Verdict, a.Why = Review, WhyUnused
		case a.Intel:
			a.Verdict, a.Why = Review, WhyIntel
		}
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].Size != apps[j].Size {
			return apps[i].Size > apps[j].Size
		}
		return apps[i].Name < apps[j].Name
	})
	return apps
}

// Source says where the app came from.
func (a AppInfo) Source() string {
	switch {
	case a.Cask != "":
		return "brew"
	case a.Store:
		return "App Store"
	}
	return ""
}

// trashPath is a free name in the Trash for something being removed.
func trashPath(name string, now time.Time) string {
	dest := filepath.Join(Home, ".Trash", name)
	if pathExists(dest) {
		ext := filepath.Ext(name)
		dest = filepath.Join(Home, ".Trash", strings.TrimSuffix(name, ext)+" "+now.Format("15.04.05")+ext)
	}
	return dest
}

// restoreCommand puts something back from the Trash. It fails when the
// place is taken again (the app was reinstalled, or it made its folder
// anew): mv would put the old folder inside the new one, or with -n do
// nothing, and both end with status 0, which reads as "restored".
func restoreCommand(from, to string) string {
	return fmt.Sprintf("test ! -e %s && mv -n %s %s", shellQuote(to), shellQuote(from), shellQuote(to))
}

// UninstallChange removes the app: through Homebrew when a cask installed
// it, otherwise by moving it to the Trash, where it stays until emptied.
func (a AppInfo) UninstallChange(now time.Time) Change {
	c := Change{ID: "app-remove:" + a.Bundle, Section: "apps", Title: i18n.T("mac.act.app_remove", a.Name)}
	if a.Cask != "" {
		c.Command = "brew uninstall --cask " + shellQuote(a.Cask)
		c.Undo = "brew install --cask " + shellQuote(a.Cask)
		return c
	}
	dest := trashPath(filepath.Base(a.Path), now)
	c.Command = fmt.Sprintf("mv -n %s %s", shellQuote(a.Path), shellQuote(dest))
	c.Undo = restoreCommand(dest, a.Path)
	return c
}

// Leftover is what an app that is no longer installed left in ~/Library.
type Leftover struct {
	Bundle string
	Paths  []string
	Size   int64
	Vendor bool // another app of the same vendor is installed: it may share this data
}

// leftoverDirs are the ~/Library folders where apps keep files named after
// their bundle id. Containers are left out: reading them makes macOS ask for
// the "data from other apps" permission.
var leftoverDirs = []string{"Application Support", "Caches", "Preferences", "Saved Application State", "HTTPStorages", "WebKit", "Logs", "Application Scripts"}

var (
	bundleLike      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(\.[A-Za-z0-9_-]+){2,}$`)
	leftoverSuffix  = []string{".plist", ".savedState", ".binarycookies"}
	leftoverIgnored = []string{"com.apple.", "group.", "systemgroup.", "org.python.", "org.swift.", "com.crashlytics.", "io.sentry.", "com.google.keystone"}
)

// LeftoverMin is the size under which leftovers are not worth listing.
const LeftoverMin = 5 << 20

func vendorOf(bundle string) string {
	parts := strings.SplitN(strings.ToLower(bundle), ".", 3)
	if len(parts) < 3 {
		return strings.ToLower(bundle)
	}
	return parts[0] + "." + parts[1]
}

// Leftovers finds files in ~/Library named after the bundle id of an app
// that is not installed any more, grouped by app, largest first.
func Leftovers(p *UserPolicy, installed map[string]App) []Leftover {
	var ids []string
	vendors := map[string]bool{}
	for id := range installed {
		ids = append(ids, strings.ToLower(id))
		vendors[vendorOf(id)] = true
	}
	owned := func(c string) bool {
		c = strings.ToLower(c)
		for _, pre := range leftoverIgnored {
			if strings.HasPrefix(c, pre) {
				return true
			}
		}
		for _, id := range ids {
			if c == id || strings.HasPrefix(c, id+".") || strings.HasPrefix(id, c+".") {
				return true
			}
		}
		return false
	}
	groups := map[string]*Leftover{}
	for _, dir := range leftoverDirs {
		entries, _ := os.ReadDir(filepath.Join(Home, "Library", dir))
		for _, e := range entries {
			name := e.Name()
			for _, suf := range leftoverSuffix {
				name = strings.TrimSuffix(name, suf)
			}
			if !bundleLike.MatchString(name) || owned(name) || (p != nil && match(p.Apps.Keep, name)) {
				continue
			}
			g := groups[strings.ToLower(name)]
			if g == nil {
				g = &Leftover{Bundle: name, Vendor: vendors[vendorOf(name)]}
				groups[strings.ToLower(name)] = g
			}
			g.Paths = append(g.Paths, filepath.Join(Home, "Library", dir, e.Name()))
		}
	}
	var all []*Leftover
	for _, g := range groups {
		all = append(all, g)
	}
	each(len(all), func(i int) {
		for _, path := range all[i].Paths {
			all[i].Size += diskSize(path)
		}
	})
	var out []Leftover
	for _, g := range all {
		// Spotlight is the last word: the app may live outside /Applications.
		if g.Size >= LeftoverMin && !bundleFound(g.Bundle) {
			out = append(out, *g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Size != out[j].Size {
			return out[i].Size > out[j].Size
		}
		return out[i].Bundle < out[j].Bundle
	})
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// TrashChange moves the leftovers to one folder in the Trash. Nothing is
// deleted until the user empties it, and the undo puts every file back.
func (l Leftover) TrashChange(now time.Time) Change {
	dest := trashPath(l.Bundle+" (omega)", now)
	cmd := []string{"mkdir -p " + shellQuote(dest)}
	var undo []string
	for _, p := range l.Paths {
		// Keep the parent folder in the name: several folders hold an entry
		// with the same name.
		to := filepath.Join(dest, filepath.Base(filepath.Dir(p))+" — "+filepath.Base(p))
		cmd = append(cmd, fmt.Sprintf("mv -n %s %s", shellQuote(p), shellQuote(to)))
		undo = append(undo, restoreCommand(to, p))
	}
	return Change{ID: "leftover-trash:" + l.Bundle, Section: "apps", Title: i18n.T("mac.act.leftover_trash", l.Bundle, HumanSize(l.Size)),
		Command: strings.Join(cmd, " && "), Undo: strings.Join(undo, " && ")}
}

// ReviewApps are the apps worth asking about.
func (inv *Inventory) ReviewApps() []AppInfo {
	var out []AppInfo
	for _, a := range inv.AppList {
		if a.Verdict != Keep {
			out = append(out, a)
		}
	}
	return out
}

// HumanSize formats bytes the way Finder does.
func HumanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n > 0:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return "—"
}
