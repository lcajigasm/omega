package macos

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/lcajigasm/omega/internal/config"
)

// Categories omega's defaults sort apps and background jobs into.
const (
	CatUpdater   = "updater"   // only checks for updates: does not need to run all the time
	CatEssential = "essential" // VPN, security, sync, input devices, menu bar managers, launchers
	CatOnDemand  = "on-demand" // chat, media, AI assistants, heavy suites: open them when needed
	CatSystem    = "system"    // Apple
	CatUnknown   = "unknown"   // no rule: the AI can research it
)

// Verdicts: what the policy recommends for an item.
const (
	Keep    = "keep"
	Disable = "disable"
	Review  = "review"
)

// rule matches an app or job by name, bundle id or launchd label.
type rule struct {
	Patterns []string
	Category string
	Why      string // English, for the agent; the interface uses the category's translation
}

// defaultRules are omega's built-in "sensible defaults". The first match wins.
var defaultRules = []rule{
	{Category: CatSystem, Patterns: []string{"com.apple.*"}},
	{Category: CatUpdater, Why: "only checks for updates; the app (or omega with Homebrew) updates it when it is opened",
		Patterns: []string{"com.google.keystone*", "com.google.GoogleUpdater*", "com.google.Keystone*", "com.adobe.ARMDC*", "com.adobe.ARMDCHelper*",
			"com.adobe.AdobeUpdate*", "com.adobe.agsservice", "com.microsoft.update.agent", "com.microsoft.autoupdate*", "com.oracle.java.*Updater*",
			"us.zoom.updater*", "us.zoom.ZoomDaemon", "org.mozilla.*updater*", "com.teamviewer.*updater*", "com.skype.*updater*",
			"*.SparkleUpdater*", "*.updater", "*.updater.*", "*update-agent*", "com.brave.Browser.*Updater*"}},
	{Category: CatEssential, Why: "VPN, security, sync, input devices, launchers or menu bar managers usually need to run all the time",
		Patterns: []string{
			"net.mullvad.*", "org.openvpn.*", "com.tailscale.*", "io.tailscale.*", "com.wireguard.*", "com.nordvpn.*", "com.expressvpn.*", "com.protonvpn.*", "ch.protonvpn.*",
			"com.1password.*", "com.agilebits.*", "2BUA8C4S2C.com.1password*", "com.bitwarden.*", "com.objective-see.*", "at.obdev.littlesnitch*", "com.malwarebytes.*",
			"com.getdropbox.*", "com.dropbox.*", "com.google.drivefs*", "com.microsoft.OneDrive*", "com.backblaze.*", "com.arqbackup.*", "com.carbonite.*",
			"com.logi.*", "com.logitech.*", "org.pqrs.*", "com.hegenberg.*", "com.knollsoft.Rectangle*", "com.crowdcafe.windowmagnet", "com.lwouis.alt-tab-macos",
			"com.surteesstudios.Bartender*", "com.jordanbaird.Ice", "*thaw*", "com.dwarvesv.minimalbar",
			"com.runningwithcrayons.Alfred*", "com.raycast.macos", "com.lcajigasm.omega*", "com.docker.vmnetd", "com.docker.socket",
			"cc.ffitch.shottr", "pl.maketheweb.cleanshotx", "com.choosyosx.Choosy*", "com.izual.Easydict", "com.superultra.Homerow"}},
	{Category: CatOnDemand, Why: "does not need to start with the Mac or stay in the background; open it when you need it",
		Patterns: []string{
			"com.openai.*", "com.anthropic.*", "ai.perplexity.*", "com.perplexity.*", "com.microsoft.copilot*", "com.anysphere.sand", "com.x.grok*", "com.google.gemini*",
			"com.tinyspeck.slackmacgap*", "com.spotify.*", "com.hnc.Discord*", "com.microsoft.teams*", "us.zoom.xos*", "ru.keepcoder.Telegram*", "net.whatsapp.*", "desktop.WhatsApp*",
			"com.docker.docker", "com.docker.helper", "com.valvesoftware.steam*", "com.adobe.acc.*", "com.adobe.AdobeCreativeCloud*", "com.adobe.CCXProcess*",
			"com.microsoft.edgemac*", "com.google.Chrome*", "com.brave.Browser*", "notion.id*", "com.notion.*", "com.electron.*", "com.github.GitHubClient*",
			"com.skype.*", "com.webex.*", "com.cisco.webex*", "com.nvidia.gfnpc*", "com.epicgames.*", "com.blizzard.*",
			"com.mitchellh.ghostty", "dev.warp.*", "com.todesktop.*", "com.microsoft.VSCode*", "dev.zed.*", "com.sublimetext.*", "com.exafunction.windsurf"}},
}

// UserPolicy is ~/.config/omega/mac.toml: the user's own rules, which win
// over omega's defaults. The agent adds to it when the user decides something.
type UserPolicy struct {
	Startup struct {
		Keep    []string `toml:"keep"`
		Disable []string `toml:"disable"`
	} `toml:"startup"`
	MenuBar struct {
		Keep []string `toml:"keep"`
		Hide []string `toml:"hide"`
	} `toml:"menubar"`
	Dock struct {
		Remove      []string `toml:"remove"`
		Add         []string `toml:"add"`
		Autohide    *bool    `toml:"autohide,omitempty"`
		ShowRecents *bool    `toml:"show_recents,omitempty"`
	} `toml:"dock"`
	Shortcuts struct {
		// Owners says who keeps a shortcut when several apps want it:
		// "opt+space" = "ChatGPT", "ctrl+space" = "macOS".
		Owners map[string]string `toml:"owners"`
	} `toml:"shortcuts"`
}

// PolicyPath is the user's rules file.
func PolicyPath() string { return filepath.Join(config.Dir(), "mac.toml") }

// LoadPolicy reads mac.toml; a missing file is an empty policy.
func LoadPolicy() (*UserPolicy, error) {
	p := &UserPolicy{}
	if _, err := toml.DecodeFile(PolicyPath(), p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return p, fmt.Errorf("reading %s: %w", PolicyPath(), err)
	}
	// Combos may be written in any order or with glyphs: key them the way
	// Conflicts looks them up.
	owners := map[string]string{}
	for k, v := range p.Shortcuts.Owners {
		if c, err := ParseCombo(k); err == nil {
			k = c.Config()
		}
		owners[k] = v
	}
	p.Shortcuts.Owners = owners
	return p, nil
}

// Save writes mac.toml.
func (p *UserPolicy) Save() error {
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return err
	}
	f, err := os.Create(PolicyPath())
	if err != nil {
		return err
	}
	defer f.Close()
	f.WriteString(policyHeader)
	return toml.NewEncoder(f).Encode(p)
}

const policyHeader = `# omega mac: your own rules. They win over omega's defaults.
# Patterns match an app name, a bundle id or a launchd label; * is a wildcard.
#
# [startup]  keep = ["1Password", "net.mullvad.*"]   disable = ["com.google.keystone.*"]
# [menubar]  keep = ["Dropbox"]                      hide = ["Docker"]
# [dock]     remove = ["App Store"]  add = ["/Applications/Ghostty.app"]  autohide = true  show_recents = false
# [shortcuts.owners]  "opt+space" = "ChatGPT"  "ctrl+space" = "macOS"  "cmd+space" = "Alfred"

`

// AddRule records a decision in the user's policy. kind is one of
// keep, disable (startup), menubar-keep, menubar-hide, dock-remove, dock-add,
// or owner (value is "combo=app").
func (p *UserPolicy) AddRule(kind, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("empty value")
	}
	add := func(list *[]string) {
		for _, v := range *list {
			if strings.EqualFold(v, value) {
				return
			}
		}
		*list = append(*list, value)
	}
	switch kind {
	case "keep":
		add(&p.Startup.Keep)
	case "disable":
		add(&p.Startup.Disable)
	case "menubar-keep":
		add(&p.MenuBar.Keep)
	case "menubar-hide":
		add(&p.MenuBar.Hide)
	case "dock-remove":
		add(&p.Dock.Remove)
	case "dock-add":
		add(&p.Dock.Add)
	case "owner":
		combo, app, ok := strings.Cut(value, "=")
		if !ok {
			return fmt.Errorf("owner rules look like opt+space=ChatGPT")
		}
		k, err := ParseCombo(combo)
		if err != nil {
			return err
		}
		p.Shortcuts.Owners[k.Config()] = strings.TrimSpace(app)
	default:
		return fmt.Errorf("unknown rule kind %q", kind)
	}
	return nil
}

// match reports whether any pattern matches any of the names, ignoring case.
func match(patterns []string, names ...string) bool {
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		for _, n := range names {
			n = strings.ToLower(n)
			if n == "" {
				continue
			}
			if ok, _ := path.Match(p, n); ok || p == n {
				return true
			}
		}
	}
	return false
}

// Categorize returns omega's default category for an app or job, and why.
func Categorize(names ...string) (string, string) {
	for _, r := range defaultRules {
		if match(r.Patterns, names...) {
			return r.Category, r.Why
		}
	}
	return CatUnknown, ""
}

// StartupVerdict decides what to recommend for something that opens at login
// or runs in the background. The user's rules come first.
func (p *UserPolicy) StartupVerdict(names ...string) (verdict, category, why string) {
	category, why = Categorize(names...)
	switch {
	case match(p.Startup.Keep, names...):
		return Keep, category, "the user's rules keep it"
	case match(p.Startup.Disable, names...):
		return Disable, category, "the user's rules disable it"
	}
	switch category {
	case CatUpdater, CatOnDemand:
		return Disable, category, why
	case CatUnknown:
		return Review, category, why
	}
	return Keep, category, why
}

// MenuBarVerdict decides what to recommend for a menu bar app.
func (p *UserPolicy) MenuBarVerdict(names ...string) (verdict, category, why string) {
	category, why = Categorize(names...)
	switch {
	case match(p.MenuBar.Keep, names...):
		return Keep, category, "the user's rules keep it"
	case match(p.MenuBar.Hide, names...):
		return Disable, category, "the user's rules hide it"
	}
	switch category {
	case CatUpdater:
		return Disable, category, why
	case CatUnknown, CatOnDemand:
		return Review, category, why
	}
	return Keep, category, why
}
