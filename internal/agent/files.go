package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
)

// read_file and write_file.

func toolReadFile(_ context.Context, a *Agent, in map[string]any) (string, error) {
	path := expand(getString(in, "path"))
	a.emit(Event{Kind: EvToolStart, Tool: "read_file", Command: path, Risk: safety.Safe})
	if secretPath(path) {
		msg := i18n.T("agent.secret_file", path)
		a.emit(Event{Kind: EvToolEnd, Tool: "read_file", Command: path, Failed: true, Reason: msg})
		return "", explained(errors.New(msg))
	}
	data, truncated, err := readHead(path, maxReadFile)
	if err != nil {
		a.emit(Event{Kind: EvToolEnd, Tool: "read_file", Command: path, Failed: true, Reason: err.Error()})
		return "", explained(err)
	}
	a.emit(Event{Kind: EvToolEnd, Tool: "read_file", Command: path})
	// A startup file is readable, the keys exported in it are not.
	text := maskPersonal("read "+path, runner.Scrub(string(data)))
	if truncated {
		return text + "\n[… file truncated …]", nil
	}
	return text, nil
}

// maxReadFile is how much of a file read_file hands to the model.
const maxReadFile = 64 * 1024

// readHead reads the first max bytes of a regular file. A device or a pipe
// is refused: /dev/zero never ends and a FIFO never starts, and the whole
// file was read before it was cut.
func readHead(path string, max int64) (data []byte, truncated bool, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}
	if !fi.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	data, err = io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > max {
		return data[:max], true, nil
	}
	return data, false, nil
}

// secretPath reports a file read_file must refuse: as typed, as an absolute
// path, and as what a symbolic link points at.
func secretPath(path string) bool {
	if safety.SecretFile(path) {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if safety.SecretFile(abs) {
		return true
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil && real != abs {
		return safety.SecretFile(real)
	}
	return false
}

// loginFiles are the files a shell reads when it starts.
var loginFiles = map[string]bool{".zshrc": true, ".zshenv": true, ".zprofile": true, ".zlogin": true, ".zlogout": true,
	".bashrc": true, ".bash_profile": true, ".bash_login": true, ".bash_logout": true, ".profile": true,
	"config.fish": true, "profile.ps1": true, "microsoft.powershell_profile.ps1": true}

// codeFiles are other files a program runs commands from: the ones shell
// startup files usually read, direnv, and the per-user settings of npm,
// curl and wget (a registry, a proxy). ~/.gitconfig is not here: tuning it
// is ordinary maintenance, and it is asked about like any other write.
var codeFiles = map[string]bool{".zshrc.local": true, ".bash_aliases": true, ".aliases": true,
	".p10k.zsh": true, ".envrc": true, ".direnvrc": true, ".npmrc": true, ".curlrc": true, ".wgetrc": true}

// sensitiveWrite reports a file whose content decides what runs later
// without anyone asking: at login, on a schedule, as root, or in omega
// itself. Writing one is asked about every time, also under --yes. The
// path is checked as typed, as an absolute path and as what a symbolic
// link points at.
func sensitiveWrite(path string) bool {
	forms := []string{path}
	if abs, err := filepath.Abs(path); err == nil {
		forms = append(forms, abs)
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			forms = append(forms, real)
		} else if dir, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
			forms = append(forms, filepath.Join(dir, filepath.Base(abs))) // a new file in a linked folder
		}
	}
	own := strings.ToLower(filepath.ToSlash(config.Dir()))
	state := strings.ToLower(filepath.ToSlash(config.StateDir())) // the journal `omega mac undo` runs commands from
	for _, f := range forms {
		p := strings.ToLower(filepath.ToSlash(f))
		if loginFiles[p[strings.LastIndex(p, "/")+1:]] {
			return true
		}
		if codeFiles[p[strings.LastIndex(p, "/")+1:]] {
			return true
		}
		// ~/.aws/config runs credential_process and ~/.kube/config an exec
		// plugin: both run a program named in the file.
		if strings.HasSuffix(p, "/.aws/config") || strings.Contains(p, "/.kube/") {
			return true
		}
		for _, dir := range []string{"/.ssh/", "/launchagents/", "/launchdaemons/", "/systemd/", "/cron", "/.config/fish/", "/.config/autostart/", "/sudoers",
			"/.git/hooks/", "/.oh-my-zsh/custom/", "/.config/direnv/"} {
			if strings.Contains(p, dir) {
				return true
			}
		}
		if strings.HasPrefix(p, "/etc/") || strings.HasPrefix(p, "/private/etc/") || p == own || strings.HasPrefix(p, own+"/") ||
			p == state || strings.HasPrefix(p, state+"/") || strings.HasSuffix(p, "/.git/config") {
			return true
		}
	}
	return false
}

func toolWriteFile(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	path := expand(getString(in, "path"))
	content, _ := in["content"].(string)
	reason := getString(in, "reason")
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	// The old version is only read to show what changes: a regular file,
	// and no more of it than read_file would take. A device or a pipe in
	// that place would never finish reading.
	fi, statErr := os.Stat(path)
	exists := statErr == nil
	if exists && !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	var oldSize int64
	// A new file is shown whole (its start and its end when long): what
	// it says is what the user approves.
	preview := "(new file)\n" + diffPreview("", content)
	if exists {
		oldSize = fi.Size()
		old, truncated, err := readHead(path, maxReadFile)
		switch {
		case err != nil:
			preview = "(the current file could not be read: " + err.Error() + ")"
		case truncated:
			preview = fmt.Sprintf("(the current file is %d bytes: too large to compare)", oldSize)
		default:
			preview = diffPreview(string(old), content)
		}
	}
	summary := fmt.Sprintf("%s (%d → %d bytes)", path, oldSize, len(content))
	// Rewriting a program that exists changes what runs the next time it
	// starts: asked every time, like a startup file.
	executable := exists && fi.Mode().Perm()&0o111 != 0
	if ok, why := a.authorize(ctx, "write_file", summary, reason+"\n\n"+preview, safety.Dangerous, sensitiveWrite(path) || executable); !ok {
		return "", explained(errors.New(why))
	}
	a.emit(Event{Kind: EvToolStart, Tool: "write_file", Command: summary, Risk: safety.Dangerous})
	fail := func(err error) (string, error) {
		a.emit(Event{Kind: EvToolEnd, Tool: "write_file", Command: summary, Failed: true, Reason: err.Error()})
		return "", explained(err)
	}
	backup := ""
	if exists {
		backup = fmt.Sprintf("%s.omega-bak-%s", path, time.Now().Format("20060102-150405"))
		// The copy is as private as the original was. A second write in
		// the same second gets its own copy: the first one is the original.
		err := copyFile(path, backup, fi.Mode().Perm())
		for n := 2; errors.Is(err, os.ErrExist) && n < 100; n++ {
			backup = fmt.Sprintf("%s.omega-bak-%s-%d", path, time.Now().Format("20060102-150405"), n)
			err = copyFile(path, backup, fi.Mode().Perm())
		}
		if err != nil {
			return fail(fmt.Errorf("could not create the backup: %w", err))
		}
	}
	mode := os.FileMode(0o644)
	if exists {
		mode = fi.Mode().Perm()
	}
	if err := writeAtomic(path, []byte(content), mode); err != nil {
		return fail(err)
	}
	a.forget() // a configuration changed: what was measured before may not hold
	a.emit(Event{Kind: EvToolEnd, Tool: "write_file", Command: summary})
	if backup != "" {
		pruneBackups(path, keepBackups)
		return "written. backup at " + backup, nil
	}
	return "written (new file)", nil
}

// writeAtomic replaces a file with new content in one step: it is written
// to a temporary file in the same folder and renamed over the old one, so
// a full disk or a crash leaves the old file whole, never half of the new
// one. A symbolic link stays a link: what changes is the file it points
// to (a dotfile kept in a repository and linked from home).
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	target := path
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("%s is a link to a file that does not exist: %w", path, err)
		}
		target = real
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".omega-tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // a no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	return os.Rename(name, target)
}

// copyFile copies a file whole, whatever its size.
func copyFile(from, to string, mode os.FileMode) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

// Lines of a diff shown on the approval card: the first and the last ones.
const (
	diffHead = 25
	diffTail = 15
)

// diffPreview shows the lines that change between two versions: removed
// ones, then added ones. A long change shows its start and its end and
// says how much is in between, so what the user approves is not silently
// cut short.
func diffPreview(old, new string) string {
	ol := strings.Split(old, "\n")
	nl := strings.Split(new, "\n")
	inOld := map[string]int{}
	for _, l := range ol {
		inOld[l]++
	}
	inNew := map[string]int{}
	for _, l := range nl {
		inNew[l]++
	}
	var changed []string
	for _, l := range ol {
		if inNew[l] == 0 {
			changed = append(changed, "- "+l)
		}
	}
	for _, l := range nl {
		if inOld[l] == 0 {
			changed = append(changed, "+ "+l)
		}
	}
	if len(changed) == 0 {
		return "(no line changes)"
	}
	if len(changed) > diffHead+diffTail {
		hidden := len(changed) - diffHead - diffTail
		shown := append([]string(nil), changed[:diffHead]...)
		shown = append(shown, fmt.Sprintf("… %d more changed lines not shown …", hidden))
		changed = append(shown, changed[len(changed)-diffTail:]...)
	}
	return strings.Join(changed, "\n")
}
