package macos

import (
	"fmt"
	"os"
	posix "path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
)

// Permission is one entry of the privacy database (TCC): what System
// Settings → Privacy & Security lists under each permission.
type Permission struct {
	Service  string    // kTCCService… name
	Client   string    // bundle id, or the absolute path of a binary
	IsPath   bool      // the client is a binary, not an app bundle
	Allowed  bool      // granted (false: denied, which still takes a row)
	Target   string    // Automation: the app it may control
	Modified time.Time // zero when unknown
	System   bool      // from the system-wide database
	Name     string    // app name, or binary name with its version
	Missing  bool      // the binary or the app is no longer on this Mac
	Copies   int       // entries of this permission for a binary with the same name
	Verdict  string
	Why      string
}

// Reasons a permission is offered for removal.
const (
	WhyGone         = "gone"          // the binary's path no longer exists
	WhyNotInstalled = "not-installed" // no app with that bundle id was found
)

// tccDBs are the privacy databases: the user's, then the system-wide one.
var tccDBs = func() []string {
	return []string{
		filepath.Join(Home, "Library", "Application Support", "com.apple.TCC", "TCC.db"),
		"/Library/Application Support/com.apple.TCC/TCC.db",
	}
}

// pathExists is replaced by tests.
var pathExists = func(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// nestedBundles are the bundle ids of helpers that live inside apps or in
// Application Support, where neither InstalledApps nor Spotlight look:
// login helpers, XPC services, agents of device software.
var nestedBundles = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{}
	var roots []string
	for _, apps := range []string{"/Applications", filepath.Join(Home, "Applications")} {
		for _, sub := range []string{"Library/LoginItems", "Library/LaunchServices", "Helpers", "Frameworks", "XPCServices", "PlugIns", "MacOS", "Resources", "SharedSupport"} {
			roots = append(roots, filepath.Join(apps, "*.app", "Contents", sub, "*"))
		}
	}
	for _, lib := range []string{"/Library/Application Support", filepath.Join(Home, "Library", "Application Support")} {
		roots = append(roots, filepath.Join(lib, "*", "*"), filepath.Join(lib, "*", "*", "*"))
	}
	for _, root := range roots {
		for _, ext := range []string{".app", ".xpc", ".appex"} {
			matches, _ := filepath.Glob(root + ext)
			for _, m := range matches {
				if id := asString(readPlist(filepath.Join(m, "Contents", "Info.plist"))["CFBundleIdentifier"]); id != "" {
					out[id] = true
				}
			}
		}
	}
	return out
})

// launchdKnows reports whether a launchd job or a privileged helper is
// named after the bundle id: the software is still there.
func launchdKnows(bundle string) bool {
	for _, p := range []string{"/Library/PrivilegedHelperTools/" + bundle, "/Library/LaunchDaemons/" + bundle + ".plist",
		"/Library/LaunchAgents/" + bundle + ".plist", filepath.Join(Home, "Library", "LaunchAgents", bundle+".plist")} {
		if pathExists(p) {
			return true
		}
	}
	return false
}

// bundleFound looks for an app that /Applications does not hold directly:
// helpers nested in other apps, device agents, and whatever Spotlight knows
// (Xcode's tools, apps kept elsewhere). Tests replace it.
var bundleFound = func(bundle string) bool {
	if strings.ContainsAny(bundle, `'"\`) {
		return true // not a real bundle id: do not suggest removing it
	}
	if nestedBundles()[bundle] || launchdKnows(bundle) {
		return true
	}
	out, err := Output("mdfind", "kMDItemCFBundleIdentifier == '"+bundle+"'")
	if err != nil {
		return true // Spotlight is off: assume the app exists
	}
	return strings.TrimSpace(string(out)) != ""
}

const tccQuery = "select service, client, client_type, auth_value, coalesce(indirect_object_identifier,''), coalesce(last_modified,0) from access"

// readTCC reads one privacy database. ok is false when it cannot be opened:
// the user's database needs Full Disk Access for the terminal.
func readTCC(db string, system bool) (perms []Permission, ok bool) {
	out, err := Output("sqlite3", "-readonly", "-separator", "\x1f", db, tccQuery)
	if err != nil {
		return nil, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) < 6 || f[1] == "" {
			continue
		}
		auth, _ := strconv.Atoi(f[3])
		p := Permission{Service: f[0], Client: f[1], IsPath: f[2] == "1", Allowed: auth >= 2, System: system}
		if f[4] != "UNUSED" {
			p.Target = f[4]
		}
		if n, _ := strconv.ParseInt(f[5], 10, 64); n > 0 {
			p.Modified = time.Unix(n, 0)
		}
		perms = append(perms, p)
	}
	return perms, true
}

// pathVersion matches a folder named after a version: 2.0.14, v20.11.1, node@24.1.
var pathVersion = regexp.MustCompile(`^(?:.*[@_-])?v?(\d+\.\d+(?:\.\d+)*)$`)

// binaryName is a binary's name with the version found in its path, which is
// what tells apart the copies System Settings lists under one name.
func binaryName(path string) string {
	// These are paths from the Mac's database: always with forward slashes,
	// whatever system the code is built for.
	name := posix.Base(path)
	if i := strings.Index(path, ".app/"); i > 0 {
		app := strings.TrimSuffix(posix.Base(path[:i+4]), ".app")
		if !strings.EqualFold(app, name) {
			name += " (" + app + ")"
		}
	}
	// The folder closest to the binary names its version: python@3.13/3.13.2/bin.
	parts := strings.Split(posix.Dir(path), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if m := pathVersion.FindStringSubmatch(parts[i]); m != nil {
			return name + " " + m[1]
		}
	}
	return name
}

// Permissions reads the privacy databases and marks what is left over: a
// binary whose path is gone (every update of a bundled node leaves one) or
// an app that is no longer installed. noAccess is true when the user's
// database could not be read.
func Permissions(p *UserPolicy, apps map[string]App) (perms []Permission, noAccess bool) {
	for i, db := range tccDBs() {
		rows, ok := readTCC(db, i > 0)
		if !ok && i == 0 {
			noAccess = true
		}
		perms = append(perms, rows...)
	}
	return judgePermissions(p, perms, apps), noAccess
}

func judgePermissions(p *UserPolicy, perms []Permission, apps map[string]App) []Permission {
	found := map[string]bool{}
	copies := map[string]int{}
	for _, x := range perms {
		if x.IsPath {
			copies[x.Service+"\x00"+filepath.Base(x.Client)]++
		}
	}
	for i := range perms {
		x := &perms[i]
		x.Verdict = Keep
		if x.IsPath {
			x.Name = binaryName(x.Client)
			x.Copies = copies[x.Service+"\x00"+filepath.Base(x.Client)]
			if !pathExists(x.Client) {
				x.Missing, x.Verdict, x.Why = true, Disable, WhyGone
			}
		} else {
			x.Name = x.Client
			if a, ok := apps[x.Client]; ok {
				x.Name = a.Name
			} else if !strings.HasPrefix(x.Client, "com.apple.") {
				ok, seen := found[x.Client]
				if !seen {
					ok = bundleFound(x.Client)
					found[x.Client] = ok
				}
				if !ok {
					x.Missing, x.Verdict, x.Why = true, Review, WhyNotInstalled
				}
			}
		}
		if x.Verdict != Keep && p != nil && match(p.Permissions.Keep, x.Client, x.Name, filepath.Base(x.Client)) {
			x.Verdict, x.Why = Keep, "the user's rules keep it"
		}
	}
	sort.SliceStable(perms, func(i, j int) bool {
		a, b := perms[i], perms[j]
		if a.Service != b.Service {
			return ServiceName(a.Service) < ServiceName(b.Service)
		}
		if a.Missing != b.Missing {
			return a.Missing
		}
		if fa, fb := filepath.Base(a.Client), filepath.Base(b.Client); fa != fb {
			return fa < fb
		}
		return a.Modified.Before(b.Modified)
	})
	return perms
}

// serviceNames are the names System Settings uses for each permission.
var serviceNames = map[string]string{
	"SystemPolicyAllFiles":          "Full Disk Access",
	"SystemPolicyDesktopFolder":     "Files & Folders: Desktop",
	"SystemPolicyDocumentsFolder":   "Files & Folders: Documents",
	"SystemPolicyDownloadsFolder":   "Files & Folders: Downloads",
	"SystemPolicyNetworkVolumes":    "Files & Folders: Network Volumes",
	"SystemPolicyRemovableVolumes":  "Files & Folders: Removable Volumes",
	"SystemPolicyAppBundles":        "App Management",
	"SystemPolicyAppData":           "Data from Other Apps",
	"SystemPolicySysAdminFiles":     "Administer the Computer",
	"SystemPolicyDeveloperFiles":    "Developer Files",
	"FileProviderDomain":            "Files & Folders: Cloud Storage",
	"Accessibility":                 "Accessibility",
	"PostEvent":                     "Accessibility (send keystrokes)",
	"ListenEvent":                   "Input Monitoring",
	"ScreenCapture":                 "Screen & System Audio Recording",
	"AudioCapture":                  "System Audio Recording",
	"AppleEvents":                   "Automation",
	"DeveloperTool":                 "Developer Tools",
	"Camera":                        "Camera",
	"Microphone":                    "Microphone",
	"AddressBook":                   "Contacts",
	"Calendar":                      "Calendars",
	"Reminders":                     "Reminders",
	"Photos":                        "Photos",
	"PhotosAdd":                     "Photos (add only)",
	"MediaLibrary":                  "Media & Apple Music",
	"BluetoothAlways":               "Bluetooth",
	"SpeechRecognition":             "Speech Recognition",
	"Motion":                        "Motion & Fitness",
	"FocusStatus":                   "Focus",
	"Liverpool":                     "iCloud",
	"Ubiquity":                      "iCloud Drive",
	"WebBrowserPublicKeyCredential": "Passkeys Access for Web Browsers",
	"EndpointSecurityClient":        "Endpoint Security",
}

// servicePanes are the anchors of the Privacy & Security pane.
var servicePanes = map[string]string{
	"SystemPolicyAllFiles": "Privacy_AllFiles", "Accessibility": "Privacy_Accessibility", "PostEvent": "Privacy_Accessibility",
	"ListenEvent": "Privacy_ListenEvent", "ScreenCapture": "Privacy_ScreenCapture", "AppleEvents": "Privacy_Automation",
	"DeveloperTool": "Privacy_DevTools", "SystemPolicyAppBundles": "Privacy_AppBundles", "Camera": "Privacy_Camera",
	"Microphone": "Privacy_Microphone", "SystemPolicyDesktopFolder": "Privacy_FilesAndFolders",
	"SystemPolicyDocumentsFolder": "Privacy_FilesAndFolders", "SystemPolicyDownloadsFolder": "Privacy_FilesAndFolders",
	"SystemPolicyNetworkVolumes": "Privacy_FilesAndFolders", "SystemPolicyRemovableVolumes": "Privacy_FilesAndFolders",
}

// ShortService drops the kTCCService prefix: the name tccutil takes.
func ShortService(service string) string { return strings.TrimPrefix(service, "kTCCService") }

// ServiceName is the permission's name in System Settings.
func ServiceName(service string) string {
	s := ShortService(service)
	if n, ok := serviceNames[s]; ok {
		return n
	}
	return s
}

// PrivacyPane is the command that opens the settings pane of a permission.
func PrivacyPane(service string) string {
	anchor := servicePanes[ShortService(service)]
	if anchor == "" {
		anchor = "Privacy"
	}
	return openSettings("com.apple.preference.security?" + anchor)
}

// ID identifies the entry in change ids: Service:client.
func (x Permission) ID() string { return ShortService(x.Service) + ":" + x.Client }

// Label is the entry as shown on a card: who, and which permission.
func (x Permission) Label() string { return x.Name + " · " + ServiceName(x.Service) }

// ManualOnly reports whether only System Settings can remove the entry. The
// system-wide database is protected by macOS, and tccutil, the one tool
// allowed to edit it, takes only the bundle id of an app that is installed:
// it refuses a binary's path and an app that is gone, which are exactly the
// entries left over.
func (x Permission) ManualOnly() bool { return x.System && (x.IsPath || x.Missing) }

// ResetChange removes the entry from the privacy database. It cannot be
// undone: the app or binary asks again the next time it needs the permission.
func (x Permission) ResetChange() Change {
	if x.ManualOnly() {
		return Change{ID: "perm-reset:" + x.ID(), Section: "permissions", Title: i18n.T("mac.act.perm_reset", x.Name, ServiceName(x.Service)),
			Command: PrivacyPane(x.Service), Manual: i18n.T("mac.manual.perm_system", x.Name, ServiceName(x.Service))}
	}
	return Change{ID: "perm-reset:" + x.ID(), Section: "permissions", Title: i18n.T("mac.act.perm_reset", x.Name, ServiceName(x.Service)),
		Command: fmt.Sprintf("%s mac op perm-reset %s %s", omegaBin(), shellQuote(ShortService(x.Service)), shellQuote(x.Client))}
}

// ResetServiceChange forgets every decision of one permission, for all apps.
func ResetServiceChange(service string) Change {
	s := ShortService(service)
	return Change{ID: "perm-reset-all:" + s, Section: "permissions", Title: i18n.T("mac.act.perm_reset_all", ServiceName(service)),
		Command: "tccutil reset " + shellQuote(s)}
}

// StalePermissions are the entries worth removing.
func (inv *Inventory) StalePermissions() []Permission {
	var out []Permission
	for _, x := range inv.Permissions {
		if x.Verdict != Keep {
			out = append(out, x)
		}
	}
	return out
}

// permissionListed reports whether the entry is still in a database, and
// in which one (index into tccDBs; -1 when it is gone).
func permissionListed(service, client string) (db int, readable bool) {
	db = -1
	for i, path := range tccDBs() {
		rows, ok := readTCC(path, i > 0)
		readable = readable || ok
		for _, r := range rows {
			if ShortService(r.Service) == service && r.Client == client && db < 0 {
				db = i
			}
		}
	}
	return db, readable
}

func sqlString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// tccBackup copies the user's privacy database to omega's state folder, once
// a day, before omega deletes a row from it.
func tccBackup(db string) (string, error) {
	dest := filepath.Join(config.StateDir(), "tcc-backup-"+time.Now().Format("2006-01-02")+".db")
	if pathExists(dest) {
		return dest, nil
	}
	if err := os.MkdirAll(config.StateDir(), 0o755); err != nil {
		return "", err
	}
	// sqlite's own backup gives a consistent copy of a database in use.
	if _, err := Output("sqlite3", "-readonly", db, ".backup "+sqlString(dest)); err != nil {
		return "", err
	}
	return dest, os.Chmod(dest, 0o600)
}

// ResetPermission removes one entry of the privacy database and checks that
// it is gone. It is what `omega mac op perm-reset` runs.
//
// tccutil comes first: it is Apple's tool for this. But it only accepts a
// bundle id that macOS still knows, so it refuses exactly the entries worth
// removing: a binary's path, or an app that was uninstalled. For those, the
// row is deleted from the user's database, after a copy is saved. That needs
// Full Disk Access for the terminal. The system-wide database (Full Disk
// Access, Accessibility, Screen Recording, Input Monitoring) is protected by
// macOS and can only be changed in System Settings: then the error says where.
func ResetPermission(service, client string) error {
	service = ShortService(service)
	Output("tccutil", "reset", service, client)
	db, readable := permissionListed(service, client)
	if db < 0 {
		if !readable {
			fmt.Println(i18n.T("mac.perm_unverified"))
		}
		return nil
	}
	if db == 0 {
		path := tccDBs()[0]
		if backup, err := tccBackup(path); err == nil {
			query := fmt.Sprintf("delete from access where service = %s and client = %s", sqlString("kTCCService"+service), sqlString(client))
			if _, err := Output("sqlite3", path, query); err == nil {
				// The privacy daemon keeps its own copy in memory; launchd starts it again.
				Output("killall", "tccd")
				if still, _ := permissionListed(service, client); still < 0 {
					fmt.Println(i18n.T("mac.perm_deleted", backup))
					return nil
				}
			}
		}
	}
	Output("/bin/sh", "-c", PrivacyPane(service))
	return fmt.Errorf("%s", i18n.T("mac.perm_still", client, ServiceName(service)))
}
