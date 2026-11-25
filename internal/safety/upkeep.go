package safety

import (
	"regexp"
	"strings"
)

// NotUpkeep says what a command would change that is not upkeep: how the
// user set up the machine. An unattended run (omega unattended) refuses
// such a command from the model; it returns "" for a command that only
// updates, cleans or reads. It reads the command for its intent, so it is
// written for the commands models write, not as a sandbox: what it misses
// still goes through the rest of the policy.
func NotUpkeep(command string) string {
	c := strings.ToLower(command)
	for _, r := range notUpkeepRules {
		if r.match(c) {
			return r.what
		}
	}
	return ""
}

type upkeepRule struct {
	what string
	re   []*regexp.Regexp
	// unless, when it matches, lets the command through: the read-only or
	// formula-only form of a command the rule is about.
	unless *regexp.Regexp
}

func (r upkeepRule) match(c string) bool {
	if r.unless != nil && r.unless.MatchString(c) {
		return false
	}
	for _, re := range r.re {
		if re.MatchString(c) {
			return true
		}
	}
	return false
}

// cmdStart is where a command begins inside a line: the start, or after a
// separator, a subshell or sudo.
const cmdStart = `(?:^|[;&|(\x60]|\$\(|\bsudo\s+|\bexec\s+|\bxargs\s+(?:-\S+\s+)*)\s*`

func rx(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		out[i] = regexp.MustCompile(p)
	}
	return out
}

var notUpkeepRules = []upkeepRule{
	{what: "what starts at login or runs in the background (login items, launch agents and daemons)", re: rx(
		cmdStart+`launchctl\s+(?:unload|load|bootout|bootstrap|disable|enable|remove|kill|kickstart)\b`,
		cmdStart+`sfltool\b`,
		`login ?items?\b`, `smappservice`, `backgroundtaskmanagement`,
		`library/launch(?:agents|daemons)`,
		cmdStart+`systemctl\s+(?:--user\s+)?(?:enable|disable|mask|unmask|stop|restart)\b`,
		cmdStart+`crontab\s+(?:-[er]\b|[^-\s])`,
	)},
	{what: "the menu bar", re: rx(
		`com\.apple\.controlcenter`, `menuextra`, `systemuiserver`, `nsstatusitem`, `\bbartender\b`,
	)},
	{what: "the apps that are open (quitting or killing them)", re: rx(
		cmdStart+`(?:killall|pkill)\s`,
		cmdStart+`kill\s+(?:-\S+\s+)*(?:\d|%|\$)`,
		`osascript\b.*\bquit\b`,
	)},
	{what: "the installed apps (installing, removing or reinstalling them, or upgrading apps that may be open)", re: rx(
		cmdStart+`brew\s+(?:uninstall|remove|rm|zap|reinstall|install|untap|unlink)\b`,
		cmdStart+`brew\s+upgrade\b`,
		cmdStart+`mas\s+(?:upgrade|uninstall|install|purchase|get|lucky)\b`,
		cmdStart+`mo\s+(?:uninstall|optimize)\b`,
		`(?:^|\s)(?:rm|mv|trash|srm)\s(?:[^;&|]*[\s=])?["']?(?:~|\$home|/users/[^/\s]+)?/applications/`,
		cmdStart+`(?:npm|pnpm|yarn|bun)\s+(?:uninstall|remove|rm|un)\s+(?:.*\s)?(?:-g|--global)\b`,
		cmdStart+`(?:pipx|cargo|gem)\s+uninstall\b`, cmdStart+`uv\s+tool\s+uninstall\b`,
		cmdStart+`(?:apt|apt-get|dnf|yum|pacman|zypper|snap|flatpak|winget|choco|scoop)\s+(?:remove|purge|uninstall|erase|-r\b|-rs\b)`,
	), unless: regexp.MustCompile(`brew\s+upgrade\s+(?:[^;&|]*\s)?--formula\b`)},
	{what: "the installed apps (topgrade upgrades them with its own steps, which quit open apps and remove login items)",
		re: rx(cmdStart + `topgrade\b`), unless: regexp.MustCompile(`topgrade\b[^;&|]*--dry-run`)},
	{what: "a setting of the system (power, network, firewall, FileVault, Gatekeeper, Time Machine, the login shell, default apps)", re: rx(
		cmdStart+`pmset\b`, cmdStart+`systemsetup\b`, cmdStart+`scutil\s+--set\b`, cmdStart+`networksetup\s+-set`,
		cmdStart+`spctl\s+--(?:master|global|enable|disable|add|remove)`, `socketfilterfw\s+--set`,
		cmdStart+`fdesetup\b`, cmdStart+`nvram\b`, cmdStart+`dscl\b`, cmdStart+`chsh\b`, cmdStart+`duti\b`,
		cmdStart+`tmutil\s+(?:enable|disable|setdestination|removedestination|addexclusion|removeexclusion|delete)`,
		cmdStart+`softwareupdate\s+--schedule`, cmdStart+`sysctl\s+-w\b`, `lsregister\b.*-(?:kill|delete|u)\b`,
		cmdStart+`ufw\s+(?:enable|disable|allow|deny|delete|reset)`,
	)},
	{what: "the preferences of the system or of an app", re: rx(
		cmdStart+`defaults\s+(?:-currenthost\s+)?(?:write|delete|import|rename)\b`,
		`plistbuddy`, cmdStart+`plutil\s+-(?:replace|remove|insert)`,
		cmdStart+`(?:gsettings|dconf)\s+(?:set|reset|write|load)\b`, `kwriteconfig`,
	)},
	{what: "the user's configuration (a dotfile, or a tool's global setting)", re: rx(
		`(?:>>?|\btee\s+(?:-a\s+)?)\s*["']?(?:~|\$home|/users/[^/\s]+|/home/[^/\s]+)/\.`,
		cmdStart+`sed\s+(?:-\S+\s+)*-i[^;&|]*(?:~|\$home|/users/[^/\s]+|/home/[^/\s]+)/\.`,
		cmdStart+`git\s+config\s+--(?:global|system)\s+(?:--(?:add|unset|unset-all|replace-all|remove-section|rename-section)\b|[a-z][\w.-]*\s+\S)`,
		cmdStart+`(?:npm|pnpm|yarn)\s+config\s+(?:set|delete|rm)\b`, cmdStart+`pip3?\s+config\s+(?:set|unset)\b`,
		cmdStart+`mise\s+(?:use\s+(?:-g|--global)|settings\s+set)\b`, cmdStart+`(?:asdf|pyenv|rbenv|nodenv)\s+global\b`,
		cmdStart+`nvm\s+alias\s+default\b`, cmdStart+`rustup\s+default\b`, cmdStart+`conda\s+config\s+--(?:set|add|remove)`,
		cmdStart+`gh\s+config\s+set\b`, cmdStart+`brew\s+analytics\s+(?:on|off)\b`,
	)},
	{what: "the Trash (emptying it deletes the user's files for good)", re: rx(
		`(?:^|\s)(?:rm|find)\s[^;&|]*(?:~|\$home|/users/[^/\s]+)/\.trash\b`, `empty (?:the )?trash`,
	)},
}
