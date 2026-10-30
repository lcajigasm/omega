package macos

import (
	"fmt"
	"sort"
	"strings"
)

// Mod is a set of modifier keys.
type Mod uint8

const (
	ModCtrl Mod = 1 << iota
	ModOpt
	ModShift
	ModCmd
	ModFn
)

// Combo is a key with its modifiers. Key is a macOS virtual key code.
type Combo struct {
	Key  int
	Mods Mod
}

type keyName struct {
	code    int
	name    string // config name, e.g. "space"
	display string // shown to the user, e.g. "Space"
}

var keyNames = []keyName{
	{0, "a", "A"}, {11, "b", "B"}, {8, "c", "C"}, {2, "d", "D"}, {14, "e", "E"}, {3, "f", "F"}, {5, "g", "G"}, {4, "h", "H"},
	{34, "i", "I"}, {38, "j", "J"}, {40, "k", "K"}, {37, "l", "L"}, {46, "m", "M"}, {45, "n", "N"}, {31, "o", "O"}, {35, "p", "P"},
	{12, "q", "Q"}, {15, "r", "R"}, {1, "s", "S"}, {17, "t", "T"}, {32, "u", "U"}, {9, "v", "V"}, {13, "w", "W"}, {7, "x", "X"},
	{16, "y", "Y"}, {6, "z", "Z"},
	{29, "0", "0"}, {18, "1", "1"}, {19, "2", "2"}, {20, "3", "3"}, {21, "4", "4"}, {23, "5", "5"}, {22, "6", "6"}, {26, "7", "7"},
	{28, "8", "8"}, {25, "9", "9"},
	{49, "space", "Space"}, {36, "return", "↩"}, {48, "tab", "⇥"}, {53, "esc", "⎋"}, {51, "delete", "⌫"}, {117, "forwarddelete", "⌦"},
	{123, "left", "←"}, {124, "right", "→"}, {126, "up", "↑"}, {125, "down", "↓"},
	{115, "home", "↖"}, {119, "end", "↘"}, {116, "pageup", "⇞"}, {121, "pagedown", "⇟"},
	{50, "`", "`"}, {27, "-", "-"}, {24, "=", "="}, {33, "[", "["}, {30, "]", "]"}, {42, "\\", "\\"}, {41, ";", ";"}, {39, "'", "'"},
	{43, ",", ","}, {47, ".", "."}, {44, "/", "/"},
	{122, "f1", "F1"}, {120, "f2", "F2"}, {99, "f3", "F3"}, {118, "f4", "F4"}, {96, "f5", "F5"}, {97, "f6", "F6"}, {98, "f7", "F7"},
	{100, "f8", "F8"}, {101, "f9", "F9"}, {109, "f10", "F10"}, {103, "f11", "F11"}, {111, "f12", "F12"}, {105, "f13", "F13"},
	{107, "f14", "F14"}, {113, "f15", "F15"}, {106, "f16", "F16"}, {64, "f17", "F17"}, {79, "f18", "F18"}, {80, "f19", "F19"},
}

var (
	byCode = map[int]keyName{}
	byName = map[string]int{}
)

func init() {
	for _, k := range keyNames {
		byCode[k.code] = k
		byName[k.name] = k.code
	}
	for alias, code := range map[string]int{"spacebar": 49, "enter": 36, "escape": 53, "backspace": 51, "←": 123, "→": 124, "↑": 126, "↓": 125, "↩": 36} {
		byName[alias] = code
	}
}

var modNames = []struct {
	mod     Mod
	name    string
	glyph   string
	aliases []string
}{
	{ModCtrl, "ctrl", "⌃", []string{"control", "ctl", "⌃", "^"}},
	{ModOpt, "opt", "⌥", []string{"option", "alt", "⌥"}},
	{ModShift, "shift", "⇧", []string{"⇧"}},
	{ModCmd, "cmd", "⌘", []string{"command", "super", "⌘"}},
	{ModFn, "fn", "fn", []string{"globe"}},
}

// String shows the combo the way macOS menus do, e.g. ⌃⌥Space.
func (c Combo) String() string {
	var b strings.Builder
	for _, m := range modNames {
		if c.Mods&m.mod != 0 {
			b.WriteString(m.glyph)
		}
	}
	if k, ok := byCode[c.Key]; ok {
		b.WriteString(k.display)
	} else {
		fmt.Fprintf(&b, "key%d", c.Key)
	}
	return b.String()
}

// Config is the combo as written in mac.toml, e.g. ctrl+opt+space.
func (c Combo) Config() string {
	var parts []string
	for _, m := range modNames {
		if c.Mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	if k, ok := byCode[c.Key]; ok {
		parts = append(parts, k.name)
	} else {
		parts = append(parts, fmt.Sprintf("key%d", c.Key))
	}
	return strings.Join(parts, "+")
}

// ParseCombo reads "cmd+shift+4", "opt+space" or "⌃⌥Space".
func ParseCombo(s string) (Combo, error) {
	s = strings.TrimSpace(s)
	var c Combo
	// Leading glyphs, as macOS writes them.
	for {
		found := false
		for _, m := range modNames {
			if strings.HasPrefix(s, m.glyph) && m.glyph != "fn" {
				c.Mods |= m.mod
				s = strings.TrimPrefix(s, m.glyph)
				found = true
			}
		}
		if !found {
			break
		}
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '+' })
	if len(parts) == 0 {
		return c, fmt.Errorf("no key in %q", s)
	}
	for _, p := range parts[:len(parts)-1] {
		m, ok := parseMod(p)
		if !ok {
			return c, fmt.Errorf("unknown modifier %q", p)
		}
		c.Mods |= m
	}
	key := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
	code, ok := byName[key]
	if !ok {
		return c, fmt.Errorf("unknown key %q", key)
	}
	c.Key = code
	return c, nil
}

func parseMod(p string) (Mod, bool) {
	p = strings.ToLower(strings.TrimSpace(p))
	for _, m := range modNames {
		if p == m.name {
			return m.mod, true
		}
		for _, a := range m.aliases {
			if p == a {
				return m.mod, true
			}
		}
	}
	return 0, false
}

// NSEvent modifier flags, used by com.apple.symbolichotkeys and MASShortcut.
const (
	nsShift = 1 << 17
	nsCtrl  = 1 << 18
	nsOpt   = 1 << 19
	nsCmd   = 1 << 20
	nsFn    = 1 << 23
)

func fromNS(f int) Mod {
	var m Mod
	for flag, mod := range map[int]Mod{nsShift: ModShift, nsCtrl: ModCtrl, nsOpt: ModOpt, nsCmd: ModCmd, nsFn: ModFn} {
		if f&flag != 0 {
			m |= mod
		}
	}
	return m
}

func (m Mod) ns() int {
	f := 0
	for flag, mod := range map[int]Mod{nsShift: ModShift, nsCtrl: ModCtrl, nsOpt: ModOpt, nsCmd: ModCmd, nsFn: ModFn} {
		if m&mod != 0 {
			f |= flag
		}
	}
	return f
}

// Carbon modifier flags, used by the KeyboardShortcuts library many apps embed.
const (
	carbonCmd   = 256
	carbonShift = 512
	carbonOpt   = 2048
	carbonCtrl  = 4096
)

func fromCarbon(f int) Mod {
	var m Mod
	for flag, mod := range map[int]Mod{carbonCmd: ModCmd, carbonShift: ModShift, carbonOpt: ModOpt, carbonCtrl: ModCtrl} {
		if f&flag != 0 {
			m |= mod
		}
	}
	return m
}

func (m Mod) carbon() int {
	f := 0
	for flag, mod := range map[int]Mod{carbonCmd: ModCmd, carbonShift: ModShift, carbonOpt: ModOpt, carbonCtrl: ModCtrl} {
		if m&mod != 0 {
			f |= flag
		}
	}
	return f
}

// ascii is the character symbolichotkeys stores next to the key code, or
// 65535 for keys without one.
func (c Combo) ascii() int {
	k, ok := byCode[c.Key]
	if !ok {
		return 65535
	}
	switch {
	case k.name == "space":
		return 32
	case len(k.name) == 1:
		return int(k.name[0])
	}
	return 65535
}

// alternatives suggests free combos close to c, most natural first.
func alternatives(c Combo, used map[string]bool, n int) []Combo {
	var mods []Mod
	add := func(m Mod) {
		if m == 0 || m == c.Mods {
			return
		}
		for _, x := range mods {
			if x == m {
				return
			}
		}
		mods = append(mods, m)
	}
	add(c.Mods | ModShift)
	add(c.Mods | ModCtrl)
	add(c.Mods | ModOpt)
	add(ModCtrl | ModOpt)
	add(ModCtrl | ModOpt | ModCmd)
	add(ModOpt | ModShift | ModCmd)
	add(ModCtrl | ModShift | ModCmd)
	var out []Combo
	for _, m := range mods {
		alt := Combo{Key: c.Key, Mods: m}
		if !used[alt.Config()] && !reserved[alt.Config()] {
			out = append(out, alt)
		}
		if len(out) == n {
			break
		}
	}
	return out
}

// reserved are combos apps rely on that are not global hotkeys, so omega
// never suggests them as an alternative.
var reserved = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range []string{"cmd+q", "cmd+w", "cmd+h", "cmd+m", "cmd+tab", "cmd+space", "cmd+opt+esc", "ctrl+cmd+q", "ctrl+cmd+f",
		"ctrl+cmd+space", "cmd+shift+3", "cmd+shift+4", "cmd+shift+5", "cmd+opt+h", "cmd+opt+m", "cmd+,", "cmd+shift+/"} {
		if c, err := ParseCombo(s); err == nil {
			m[c.Config()] = true
		}
	}
	return m
}()

func sortedKeys(m map[string][]Hotkey) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
