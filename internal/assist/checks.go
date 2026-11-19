package assist

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"howett.net/plist"
)

// Builders shared by the tools' definitions.

// budget is a folder the tool never trims. With oldest set, enforcing the
// budget removes the oldest files until it fits (conversations, sessions);
// without it, the folder is emptied (caches, a VM image downloaded again).
func budget(e *env, id, title, why, root string, oldest bool, dirs ...string) Check {
	n := size(dirs...)
	c := Check{ID: id, Title: title, Why: why, Kind: KindBudget, Disk: true, Size: n, Unit: "GB", OK: true}
	c.Current = "no budget"
	limit := int64(0)
	if v := e.budgets[id]; v != "" {
		if b, err := ParseSize(v); err == nil {
			limit = b
			c.Current = "budget " + Human(b)
			if n > b {
				c.OK = false
				if oldest {
					c.Free = n - b
				} else {
					c.Free = n
				}
			}
		}
	}
	if oldest {
		c.Note = "Over budget, the oldest files go first until it fits."
	} else {
		c.Note = "Over budget, the folder is emptied; the app fills it again when it needs it."
	}
	c.set = func(v string) (string, error) { return saveBudget(id, v) }
	c.clean = func() (string, error) {
		var freed int64
		var err error
		if oldest {
			if limit == 0 {
				return "", fmt.Errorf("%s has no budget: set one first (omega disk limits set %s 2GB)", id, id)
			}
			freed, err = removeOldest(root, limit, dirs...)
		} else {
			freed, err = emptyDirs(root, dirs...)
		}
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s: freed %s", title, Human(freed)), nil
	}
	return c
}

// cleanup frees paths that hold nothing anyone loses.
func cleanup(id, title, why, root string, paths []string) Check {
	n := size(paths...)
	c := Check{ID: id, Title: title, Why: why, Kind: KindCleanup, Disk: true, Size: n, Free: n, OK: n < 1<<20}
	if c.OK {
		c.Current = "nothing to free"
	} else {
		c.Current = fmt.Sprintf("%s to free (%d item%s)", Human(n), len(paths), plural(len(paths)))
	}
	c.clean = func() (string, error) {
		freed, err := removeAll(root, paths)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s: freed %s", title, Human(freed)), nil
	}
	return c
}

// caches is the cleanup of an Electron or Chromium app's caches. It counts
// as done below 100 MB: they fill again as the app is used.
func caches(id, name, data string) Check {
	dirs := cacheDirs(data)
	n := size(dirs...)
	c := Check{ID: id, Title: name + " caches", Why: "Web caches, compiled code and GPU shaders the app builds again as it is used.", Kind: KindCleanup, Disk: true, Size: n, Free: n, OK: n < 100<<20}
	c.Current = Human(n)
	c.clean = func() (string, error) {
		freed, err := emptyDirs(data, dirs...)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s caches: freed %s", name, Human(freed)), nil
	}
	return c
}

// mcpEntry is how a tool that reads mcpServers starts omega's MCP server.
func mcpEntry(e *env) map[string]any {
	return map[string]any{"command": e.omega, "args": []string{"mcp"}}
}

// mcpJSON registers omega's MCP server under key in a JSON file.
func mcpJSON(e *env, id, name, file, key string, entry map[string]any, note string) Check {
	present := jsonGet(file, key).Exists()
	c := Check{ID: id, Title: "Give " + name + " omega's tools (MCP)", Kind: KindAction, File: file, OK: present, Note: note,
		Why: "Lets the agent check the disk, the limits and these assistants through omega, read its manual and change settings with your approval, instead of improvising commands."}
	c.Current = map[bool]string{true: "registered", false: "not registered"}[present]
	c.Recommended = "register"
	c.set = func(string) (string, error) {
		values := map[string]any{}
		for k, v := range entry {
			values[key+"/"+k] = v
		}
		return jsonSet(c.toolOf(), file, "omega's MCP server for "+name, values)
	}
	return c
}

func (c *Check) toolOf() string {
	t, _, _ := strings.Cut(c.ID, ".")
	return t
}

// days is a setting counted in days.
func days(v string, lo, hi int) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "d")))
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("%q: give a number of days from %d to %d", v, lo, hi)
	}
	return n, nil
}

// appVersion reads a macOS app's version from its Info.plist.
func appVersion(bundle string) string {
	data, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		return ""
	}
	var info struct {
		Version string `plist:"CFBundleShortVersionString"`
	}
	if _, err := plist.Unmarshal(data, &info); err != nil {
		return ""
	}
	return info.Version
}

// cliVersion asks a command for its version; the first line, trimmed.
func cliVersion(path string, args ...string) string {
	if len(args) == 0 {
		args = []string{"--version"}
	}
	out, err := run(path, args...)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(out, "\n")
	if len(line) > 60 {
		line = line[:60]
	}
	return strings.TrimSpace(line)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if exists(p) {
			return p
		}
	}
	return ""
}
