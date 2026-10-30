package macos

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
)

// Kinds of startup and background items.
const (
	KindLogin  = "login"  // opens at login (System Settings → Login Items → Open at Login)
	KindHelper = "helper" // login item helper inside an app bundle (SMAppService)
	KindAgent  = "agent"  // launchd agent: runs as the user
	KindDaemon = "daemon" // launchd daemon: runs as root
	KindTask   = "task"   // background task registered by an app
)

// Item is something that opens at login or is allowed to run in the background.
type Item struct {
	Name      string
	Developer string
	Bundle    string // app bundle id, or the parent app's
	Label     string // launchd label, when it is a launchd job
	Plist     string // launchd plist path
	Path      string // app or executable
	Kind      string
	System    bool // a daemon, or a job of the system domain: needs sudo to change
	Enabled   bool
	RunAtLoad bool
	KeepAlive bool
	Running   bool
	Missing   bool // the program it runs no longer exists
	Verdict   string
	Category  string
	Why       string
}

// ID is the stable id used on cards and by the agent.
func (it Item) ID() string {
	if it.Label != "" {
		return it.Label
	}
	if it.Bundle != "" {
		return it.Bundle
	}
	return it.Name
}

// AtLogin reports whether the item starts when the user logs in.
func (it Item) AtLogin() bool { return it.Kind == KindLogin || it.RunAtLoad }

// Background reports whether the item is a job that runs in the background.
func (it Item) Background() bool { return it.Kind != KindLogin }

// btmRecord is one item of `sfltool dumpbtm`.
type btmRecord map[string]string

var btmUID = regexp.MustCompile(`Records for UID (-?\d+)`)

// parseBTM reads `sfltool dumpbtm` output, keeping the records of the
// current user and the system-wide launchd jobs.
func parseBTM(out, user string) []btmRecord {
	var recs []btmRecord
	var cur btmRecord
	section := ""
	flush := func() {
		if cur != nil {
			cur["uid"] = section
			recs = append(recs, cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if m := btmUID.FindStringSubmatch(t); m != nil {
			flush()
			section = m[1]
			continue
		}
		if strings.HasPrefix(t, "#") && strings.HasSuffix(t, ":") && !strings.Contains(line, "    #") {
			flush()
			cur = btmRecord{}
			continue
		}
		if cur == nil {
			continue
		}
		k, v, ok := strings.Cut(t, ": ")
		if !ok {
			continue
		}
		if _, dup := cur[k]; !dup {
			cur[k] = strings.TrimSpace(v)
		}
	}
	flush()
	var keep []btmRecord
	for _, r := range recs {
		typ := r["Type"]
		if r["uid"] == user || strings.Contains(typ, "daemon") || strings.Contains(typ, "legacy agent") {
			keep = append(keep, r)
		}
	}
	return keep
}

func btmKind(typ string) string {
	switch {
	case strings.HasPrefix(typ, "app "):
		return KindLogin
	case strings.HasPrefix(typ, "login item"):
		return KindHelper
	case strings.Contains(typ, "daemon"):
		return KindDaemon
	case strings.Contains(typ, "agent"):
		return KindAgent
	case strings.HasPrefix(typ, "background tasks"):
		return KindTask
	}
	return "" // developer records, dock tiles, Quick Look and Spotlight plug-ins
}

// btmLabel turns a BTM identifier such as "16.net.mullvad.daemon" into the
// launchd label or bundle id it refers to.
func btmLabel(id string) string {
	if i := strings.IndexByte(id, '.'); i > 0 {
		if _, err := fmt.Sscanf(id[:i], "%d", new(int)); err == nil {
			return id[i+1:]
		}
	}
	return id
}

func fileURLPath(u string) string {
	if !strings.HasPrefix(u, "file://") && !strings.HasPrefix(u, "/") {
		return u
	}
	if p, err := url.Parse(u); err == nil && p.Path != "" {
		return strings.TrimSuffix(p.Path, "/")
	}
	return strings.TrimPrefix(u, "file://")
}

// launchdJob is the part of a launchd plist omega cares about.
type launchdJob struct {
	Label, Program, Plist string
	RunAtLoad, KeepAlive  bool
	System                bool
}

func readJob(p string) (launchdJob, bool) {
	m := readPlist(p)
	if m == nil {
		return launchdJob{}, false
	}
	j := launchdJob{Label: asString(m["Label"]), Plist: p, RunAtLoad: asBool(m["RunAtLoad"]), System: strings.HasPrefix(p, "/Library/LaunchDaemons")}
	switch v := m["KeepAlive"].(type) {
	case bool:
		j.KeepAlive = v
	case map[string]any:
		j.KeepAlive = len(v) > 0
	}
	j.Program = asString(m["Program"])
	if args, ok := m["ProgramArguments"].([]any); ok && len(args) > 0 && j.Program == "" {
		j.Program = asString(args[0])
	}
	if bp := asString(m["BundleProgram"]); bp != "" && j.Program == "" {
		j.Program = bp
	}
	return j, j.Label != ""
}

func launchdJobs() []launchdJob {
	var jobs []launchdJob
	for _, dir := range []string{filepath.Join(Home, "Library/LaunchAgents"), "/Library/LaunchAgents", "/Library/LaunchDaemons"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.plist"))
		for _, f := range files {
			if j, ok := readJob(f); ok {
				jobs = append(jobs, j)
			}
		}
	}
	return jobs
}

var disabledLine = regexp.MustCompile(`"([^"]+)"\s*=>\s*(disabled|true|enabled|false)`)

// launchdDisabled reads the labels disabled with `launchctl disable`.
func launchdDisabled(domain string) map[string]bool {
	out, err := Output("launchctl", "print-disabled", domain)
	if err != nil {
		return nil
	}
	m := map[string]bool{}
	for _, l := range disabledLine.FindAllStringSubmatch(string(out), -1) {
		m[l[1]] = l[2] == "disabled" || l[2] == "true"
	}
	return m
}

// launchdRunning returns the labels of the user's jobs that have a PID.
func launchdRunning() map[string]bool {
	out, err := Output("launchctl", "list")
	if err != nil {
		return nil
	}
	m := map[string]bool{}
	for _, l := range strings.Split(string(out), "\n")[1:] {
		f := strings.Fields(l)
		if len(f) == 3 && f[0] != "-" {
			m[f[2]] = true
		}
	}
	return m
}

// BTMTimeout bounds `sfltool dumpbtm`, which can take a minute when the
// background task daemon is busy.
var BTMTimeout = 90 * time.Second

// btmCache keeps the last `sfltool dumpbtm` output: it changes rarely and
// is slow to read.
func btmCache() string { return filepath.Join(config.StateDir(), "btm-cache.txt") }

// BTMCacheAge is how long a cached dump is used before reading it again.
var BTMCacheAge = time.Hour

// ForgetBTM drops the cached dump, after omega changes a login or background item.
func ForgetBTM() { os.Remove(btmCache()) }

// CachedBTM returns a recent dump when there is one, else reads it again.
func CachedBTM() (string, bool) {
	if fi, err := os.Stat(btmCache()); err == nil && time.Since(fi.ModTime()) < BTMCacheAge {
		if b, err := os.ReadFile(btmCache()); err == nil && len(b) > 0 {
			return string(b), true
		}
	}
	out, ok := DumpBTM()
	if ok && out != "" {
		if os.MkdirAll(config.StateDir(), 0o755) == nil {
			os.WriteFile(btmCache(), []byte(out), 0o600)
		}
	}
	return out, ok
}

// DumpBTM runs `sfltool dumpbtm`; ok is false when it fails or times out.
func DumpBTM() (string, bool) {
	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := Output("sfltool", "dumpbtm")
		ch <- result{out, err}
	}()
	select {
	case r := <-ch:
		return string(r.out), r.err == nil
	case <-time.After(BTMTimeout):
		return "", false
	}
}

// Items lists what opens at login and what is allowed in the background,
// with the policy's verdict on each. btm is the output of DumpBTM ("" when
// it was not available: then only launchd jobs are listed).
func Items(p *UserPolicy, btm string) []Item {
	var items []Item
	seen := map[string]bool{}
	jobs := map[string]launchdJob{}
	for _, j := range launchdJobs() {
		jobs[j.Label] = j
	}
	userOff := launchdDisabled("gui/" + uid())
	systemOff := launchdDisabled("system")
	running := launchdRunning()

	if btm != "" {
		for _, r := range parseBTM(btm, uid()) {
			kind := btmKind(r["Type"])
			if kind == "" {
				continue
			}
			it := Item{Name: r["Name"], Developer: r["Developer Name"], Kind: kind,
				Enabled: strings.Contains(r["Disposition"], "[enabled"), Path: fileURLPath(r["URL"])}
			if it.Developer == "(null)" {
				it.Developer = ""
			}
			if b := r["Bundle Identifier"]; b != "" && b != "(null)" {
				it.Bundle = b
			} else if a := strings.Trim(r["Assoc. Bundle IDs"], "[] "); a != "" {
				it.Bundle = strings.Fields(a)[0]
			}
			if kind != KindLogin {
				it.Label = btmLabel(r["Identifier"])
				if j, ok := jobs[it.Label]; ok {
					it.Plist, it.RunAtLoad, it.KeepAlive = j.Plist, j.RunAtLoad, j.KeepAlive
				} else if strings.HasSuffix(it.Path, ".plist") {
					if j, ok := readJob(it.Path); ok {
						it.Label, it.Plist, it.RunAtLoad, it.KeepAlive = j.Label, j.Plist, j.RunAtLoad, j.KeepAlive
					}
				}
				if e := r["Executable Path"]; e != "" && e != "(null)" {
					it.Path = e
				}
			}
			it.System = kind == KindDaemon
			if it.Label != "" && (userOff[it.Label] || (it.System && systemOff[it.Label])) {
				it.Enabled = false
			}
			key := kind + "|" + it.ID()
			if seen[key] || it.Name == "" {
				continue
			}
			seen[key] = true
			if it.Label != "" {
				seen["job|"+it.Label] = true
			}
			items = append(items, it)
		}
	}
	// launchd jobs that Background Task Management does not know about.
	for _, j := range jobs {
		if seen["job|"+j.Label] {
			continue
		}
		it := Item{Name: j.Label, Label: j.Label, Plist: j.Plist, Path: j.Program, RunAtLoad: j.RunAtLoad, KeepAlive: j.KeepAlive,
			Kind: KindAgent, System: j.System, Enabled: !userOff[j.Label] && !(j.System && systemOff[j.Label])}
		if j.System {
			it.Kind = KindDaemon
		}
		items = append(items, it)
	}
	for i := range items {
		it := &items[i]
		if it.Label != "" {
			it.Running = running[it.Label]
		}
		if it.Path != "" && strings.HasPrefix(it.Path, "/") {
			if _, err := os.Stat(it.Path); err != nil {
				it.Missing = true
			}
		}
		it.Verdict, it.Category, it.Why = p.StartupVerdict(it.Name, it.Bundle, it.Label)
		if it.Missing && it.Verdict != Keep {
			it.Verdict, it.Why = Disable, "the program it runs is no longer installed"
		}
		if !it.Enabled && it.Verdict == Disable {
			it.Verdict = Keep // already off: nothing to do
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return kindOrder(items[i].Kind) < kindOrder(items[j].Kind)
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items
}

func kindOrder(k string) int {
	return map[string]int{KindLogin: 0, KindHelper: 1, KindAgent: 2, KindTask: 3, KindDaemon: 4}[k]
}

// launchctlDomain is the launchd domain a job belongs to.
func (it Item) launchctlDomain() string {
	if it.System {
		return "system"
	}
	return "gui/" + uid()
}

// DisableChange stops an item from opening at login or running in the background.
func (it Item) DisableChange() Change {
	c := Change{ID: "disable:" + it.ID(), Title: i18n.T("mac.act.disable", it.Name)}
	switch {
	case it.Kind == KindLogin:
		c.Section = "login"
		c.Title = i18n.T("mac.act.login_remove", it.Name)
		c.Command = osascript(`tell application "System Events" to delete login item ` + appleScriptString(it.Name))
		if it.Path != "" {
			c.Undo = osascript(`tell application "System Events" to make login item at end with properties {path:` +
				appleScriptString(it.Path) + `, hidden:false}`)
		}
		c.Manual = i18n.T("mac.manual.login", it.Name)
	case it.Label != "" && it.Kind != KindTask:
		c.Section = "background"
		d := it.launchctlDomain()
		sudo := ""
		if it.System {
			sudo = "sudo "
		}
		target := shellQuote(d + "/" + it.Label)
		c.Command = fmt.Sprintf("%slaunchctl bootout %s 2>/dev/null; %slaunchctl disable %s", sudo, target, sudo, target)
		c.Undo = fmt.Sprintf("%slaunchctl enable %s", sudo, target)
		if it.Plist != "" {
			c.Undo += fmt.Sprintf("; %slaunchctl bootstrap %s %s 2>/dev/null; true", sudo, shellQuote(d), shellQuote(it.Plist))
		}
		if it.Kind == KindHelper {
			c.Manual = i18n.T("mac.manual.background", it.Name)
		}
	default:
		c.Section = "background"
		c.Command = openSettings(PaneLoginItems)
		c.Manual = i18n.T("mac.manual.background", it.Name)
	}
	return c
}

// EnableChange turns a disabled launchd job back on.
func (it Item) EnableChange() (Change, bool) {
	if it.Label == "" || it.Enabled || it.Kind == KindLogin || it.Kind == KindTask {
		return Change{}, false
	}
	d := it.DisableChange()
	return Change{ID: "enable:" + it.ID(), Section: d.Section, Title: i18n.T("mac.act.enable", it.Name), Command: d.Undo, Undo: d.Command}, true
}
