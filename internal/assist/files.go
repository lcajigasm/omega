package assist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"

	"github.com/lcajigasm/omega/internal/config"
)

// BackupDir is where a configuration file is copied before omega changes it.
func BackupDir() string { return filepath.Join(config.StateDir(), "backups") }

// backup copies a file that exists into BackupDir and returns the copy's
// path; "" when there was no file.
func backup(tool, path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	dir := filepath.Join(BackupDir(), tool)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(path)+"."+time.Now().Format("20060102-150405"))
	return dst, os.WriteFile(dst, data, 0o600)
}

// writeFile replaces a file through a temporary one in the same folder,
// keeping its mode, and writes the file a symbolic link points to rather
// than the link.
func writeFile(path string, data []byte) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".omega-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// change writes new contents to a tool's file after keeping a copy, and
// says what was done.
func change(tool, path string, data []byte, what string) (string, error) {
	old, _ := os.ReadFile(path)
	if bytes.Equal(old, data) {
		return fmt.Sprintf("%s: already set in %s", what, path), nil
	}
	bak, err := backup(tool, path)
	if err != nil {
		return "", fmt.Errorf("keeping a copy of %s: %w", path, err)
	}
	if err := writeFile(path, data); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("%s: written to %s", what, path)
	if bak != "" {
		msg += fmt.Sprintf(" (previous file kept at %s)", bak)
	}
	return msg, nil
}

// ---------- JSON ----------

// readJSON reads a JSON file; a missing or empty one is an empty object.
// JSON with comments (VS Code's files allow them) is refused rather than
// rewritten without them.
func readJSON(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) || len(bytes.TrimSpace(data)) == 0 {
		return []byte("{}"), nil
	}
	if err != nil {
		return nil, err
	}
	if !gjson.ValidBytes(data) {
		return nil, fmt.Errorf("%s is not plain JSON (comments or a mistake): omega leaves it to you", path)
	}
	return data, nil
}

// jsonGet reads one value; key is a path whose parts are separated by
// slashes, so keys with dots stay whole.
func jsonGet(path, key string) gjson.Result {
	data, err := readJSON(path)
	if err != nil {
		return gjson.Result{}
	}
	return gjson.GetBytes(data, jsonPath(key))
}

func jsonPath(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = strings.NewReplacer(".", `\.`, "*", `\*`, "?", `\?`).Replace(p)
	}
	return strings.Join(parts, ".")
}

// jsonSet writes values into a JSON file, keeping the order and the rest of
// what is there.
func jsonSet(tool, path, what string, values map[string]any) (string, error) {
	data, err := readJSON(path)
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		data, err = sjson.SetBytes(data, jsonPath(k), values[k])
		if err != nil {
			return "", err
		}
	}
	data = pretty.PrettyOptions(data, &pretty.Options{Indent: "  ", Width: 100})
	if !json.Valid(data) {
		return "", fmt.Errorf("the new %s would not be valid JSON: left as it was", path)
	}
	return change(tool, path, data, what)
}

// jsonAppend adds strings to an array, once each.
func jsonAppend(tool, path, key, what string, items []string) (string, error) {
	data, err := readJSON(path)
	if err != nil {
		return "", err
	}
	have := map[string]bool{}
	for _, v := range gjson.GetBytes(data, jsonPath(key)).Array() {
		have[v.String()] = true
	}
	for _, it := range items {
		if have[it] {
			continue
		}
		if data, err = sjson.SetBytes(data, jsonPath(key)+".-1", it); err != nil {
			return "", err
		}
	}
	data = pretty.PrettyOptions(data, &pretty.Options{Indent: "  ", Width: 100})
	return change(tool, path, data, what)
}

// ---------- TOML ----------

// tomlSet sets key = literal in [table] of a TOML file ("" is the top of the
// file), adding the table when it is missing. The file is edited as lines,
// so comments and order stay; the result must still parse.
func tomlSet(tool, path, table string, values [][2]string, what string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	text := string(data)
	if text != "" {
		var probe map[string]any
		if _, err := toml.Decode(text, &probe); err != nil {
			return "", fmt.Errorf("%s does not parse (%v): omega leaves it to you", path, err)
		}
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if text == "" {
		lines = nil
	}
	start, end, found := tomlTable(lines, table)
	for _, kv := range values {
		line := kv[0] + " = " + kv[1]
		if !found {
			found = true
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "["+table+"]")
			start, end = len(lines)-1, len(lines)
		}
		re := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(kv[0]) + `\s*=`)
		done := false
		for i := start + 1; i < end; i++ {
			if re.MatchString(lines[i]) {
				lines[i] = line
				done = true
				break
			}
		}
		if !done {
			// After the table's last key, before the blank lines that end it.
			at := end
			for at > start+1 && strings.TrimSpace(lines[at-1]) == "" {
				at--
			}
			lines = append(lines[:at], append([]string{line}, lines[at:]...)...)
			end++
		}
	}
	out := strings.Join(lines, "\n") + "\n"
	var probe map[string]any
	if _, err := toml.Decode(out, &probe); err != nil {
		return "", fmt.Errorf("the change would break %s (%v): left as it was", path, err)
	}
	return change(tool, path, []byte(out), what)
}

// tomlTable returns the header line of a table (-1 for the top of the
// file), the line after its last one, and whether it is there.
func tomlTable(lines []string, table string) (int, int, bool) {
	if table == "" {
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "[") {
				return -1, i, true
			}
		}
		return -1, len(lines), true
	}
	start := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "[") {
			continue
		}
		if start >= 0 {
			return start, i, true
		}
		if strings.HasPrefix(t, "[[") {
			continue
		}
		name := strings.TrimSpace(strings.Trim(strings.TrimSpace(strings.SplitN(t, "#", 2)[0]), "[]"))
		if name == table {
			start = i
		}
	}
	if start < 0 {
		return -1, -1, false
	}
	return start, len(lines), true
}

// tomlGet reads a value of a TOML file; nil when it is not set.
func tomlGet(path string, keys ...string) any {
	var m map[string]any
	if _, err := toml.DecodeFile(path, &m); err != nil {
		return nil
	}
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

// ---------- KEY=value files ----------

// envSet sets KEY=value in a file of such lines, Homebrew's brew.env.
func envSet(tool, path, key, value, what string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	var lines []string
	if len(data) > 0 {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}
	line := key + "=" + value
	done := false
	for i, l := range lines {
		if k, _, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "export ")), "="); ok && k == key {
			lines[i] = line
			done = true
		}
	}
	if !done {
		lines = append(lines, line)
	}
	return change(tool, path, []byte(strings.Join(lines, "\n")+"\n"), what)
}

func envGet(path, key string) string {
	data, _ := os.ReadFile(path)
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimPrefix(strings.TrimSpace(l), "export ")
		if k, v, ok := strings.Cut(l, "="); ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// ---------- sizes ----------

// Human writes a size the way people read it.
func Human(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

var sizeRe = regexp.MustCompile(`(?i)^\s*([0-9]+(?:[.,][0-9]+)?)\s*(b|k|kb|kib|m|mb|mib|g|gb|gib|t|tb|tib)?\s*$`)

// ParseSize reads 500MB, 8GB, 8 G or 1.5gb; a bare number is gigabytes.
func ParseSize(s string) (int64, error) {
	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("%q is not a size: write it as 500MB or 8GB", s)
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a size: write it as 500MB or 8GB", s)
	}
	mult := map[string]float64{"b": 1, "k": 1 << 10, "m": 1 << 20, "g": 1 << 30, "t": 1 << 40}[strings.ToLower(m[2] + "g")[:1]]
	return int64(n * mult), nil
}

// size is the disk a file or folder takes; what cannot be read counts zero.
// Blocks are counted where the system says them, so a sparse disk image
// (Docker.raw) counts what it really takes.
func size(paths ...string) int64 {
	var total int64
	for _, root := range paths {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if fi, err := d.Info(); err == nil && !d.IsDir() {
				total += onDisk(fi)
			}
			return nil
		})
	}
	return total
}

// ---------- processes and commands ----------

func lookPath(cmd string) (string, error) {
	if p, err := exec.LookPath(cmd); err == nil {
		return p, nil
	}
	// A GUI app gets a short PATH: look where the installers put them.
	home := homeDir()
	for _, dir := range []string{filepath.Join(home, ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".npm-global", "bin"), filepath.Join(home, ".bun", "bin")} {
		p := filepath.Join(dir, cmd)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

// processRunning reports whether a process whose command line holds pattern
// is running.
func processRunning(pattern string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-axo", "command=").Output()
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, pattern) {
			return true
		}
	}
	return false
}

// run executes a command for a version or a change, with a time limit.
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// omegaPath is the omega the other tools are told to start for its MCP
// server: the installed command when there is one, since a GUI app does not
// get the shell's PATH.
func omegaPath() string {
	if p, err := lookPath("omega"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	if exe, err := os.Executable(); err == nil && filepath.Base(exe) == "omega" {
		return exe
	}
	return "omega"
}

// ---------- budgets ----------

func loadBudgets() map[string]string {
	cfg, err := config.Load()
	if err != nil || cfg.DiskBudgets == nil {
		return map[string]string{}
	}
	return cfg.DiskBudgets
}

// saveBudget keeps a budget in omega's configuration; "off" removes it.
func saveBudget(id, value string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if cfg.DiskBudgets == nil {
		cfg.DiskBudgets = map[string]string{}
	}
	if value == "off" || value == "none" {
		delete(cfg.DiskBudgets, id)
		if err := cfg.Save(); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s: no budget, omega will not empty it", id), nil
	}
	n, err := ParseSize(value)
	if err != nil {
		return "", err
	}
	cfg.DiskBudgets[id] = Human(n)
	if err := cfg.Save(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: budget %s saved in %s; `omega disk limits enforce` empties it once it holds more", id, Human(n), config.Path()), nil
}
