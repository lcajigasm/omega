package safety

import "strings"

// PersonalSetup names the part of the user's own arrangement of the machine
// that a command rewrites: the Dock, the keyboard shortcuts, the wallpaper,
// the taskbar. It returns "" for everything else, and for commands that only
// read those settings. None of it is maintenance, so a run nobody watches
// leaves it alone.
func PersonalSetup(command string) string {
	c := strings.ToLower(command)
	any := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(c, w) {
				return true
			}
		}
		return false
	}
	writesDefaults := any("defaults write", "defaults delete", "defaults import", "plistbuddy", "plutil -replace", "plutil -remove", "plutil -insert")
	writesDesktop := any("gsettings set", "gsettings reset", "dconf write", "dconf reset", "dconf load", "kwriteconfig")
	writesRegistry := any("set-itemproperty", "new-itemproperty", "remove-itemproperty", "remove-item", "reg add", "reg delete")
	switch {
	case any("dockutil", "killall dock", " mac op dock-", " mac dock"),
		writesDefaults && any("com.apple.dock"),
		writesDesktop && any("favorite-apps", "dash-to-dock", "dash-to-panel", "plasma-org.kde.plasma.desktop-appletsrc"):
		return "the Dock"
	case writesDefaults && any("com.apple.symbolichotkeys", "nsuserkeyequivalents"),
		writesDesktop && any("keybindings", "kglobalshortcutsrc"):
		return "keyboard shortcuts"
	case any("set desktop picture", "set picture of"),
		writesDefaults && any("com.apple.wallpaper", "com.apple.desktop"),
		writesDesktop && any("picture-uri"):
		return "the wallpaper"
	case writesRegistry && any(`explorer\taskband`, `explorer\startpage`, "start_layout"),
		any("import-startlayout"):
		return "the taskbar and the Start menu"
	}
	return ""
}
