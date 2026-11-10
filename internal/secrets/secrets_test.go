package secrets

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/i18n"
)

// fakeKeyring stands in for secret-tool: a map behind the same arguments.
type fakeKeyring struct {
	keys map[string]string
	down bool // no Secret Service answers
	// warning is what a library prints on standard error with every call.
	warning string
	calls   []string
}

func (f *fakeKeyring) run(_ time.Duration, stdin string, name string, args ...string) (string, string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if name != "secret-tool" {
		return "", "", errors.New("unexpected program " + name)
	}
	if f.down {
		return "", "secret-tool: Cannot autolaunch D-Bus without X11 $DISPLAY", errors.New("exit status 1")
	}
	account := args[len(args)-1]
	switch args[0] {
	case "lookup":
		if k, ok := f.keys[account]; ok {
			// A library warning on standard error must not become part of the key.
			return k + "\n", f.warning, nil
		}
		return "", "", errors.New("exit status 1")
	case "store":
		f.keys[account] = stdin
		return "", f.warning, nil
	case "clear":
		delete(f.keys, account)
		return "", "", nil
	}
	return "", "", errors.New("unexpected arguments")
}

// linux points the package at a Linux machine with an empty config dir.
func linux(t *testing.T, secretTool bool) *fakeKeyring {
	t.Helper()
	i18n.Init("en")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "")
	f := &fakeKeyring{keys: map[string]string{}}
	oldOS, oldLook, oldRun := goos, lookPath, runTool
	goos = "linux"
	lookPath = func(bin string) (string, error) {
		if bin == "secret-tool" && secretTool {
			return "/usr/bin/secret-tool", nil
		}
		return "", errors.New("not found")
	}
	runTool = f.run
	t.Cleanup(func() { goos, lookPath, runTool = oldOS, oldLook, oldRun })
	return f
}

func TestLinuxFileFallback(t *testing.T) {
	f := linux(t, false)
	if _, _, err := Get("anthropic"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nothing stored yet: %v", err)
	}
	src, err := Set("anthropic", "sk-ant-file-0001")
	if err != nil || src != FromFile {
		t.Fatalf("without secret-tool the key goes to the file: %s %v", src, err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("secret-tool is not installed and must not be run: %v", f.calls)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(FilePath())
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("the key file must be 0600: %v %v", fi, err)
		}
	}
	if _, err := Set("openai", "sk-openai-0002"); err != nil {
		t.Fatal(err)
	}
	if Store() != FromFile {
		t.Fatalf("Store = %s", Store())
	}
	if runtime.GOOS != "windows" {
		os.Chmod(FilePath(), 0o644)
		Get("openai")
		if fi, _ := os.Stat(FilePath()); fi.Mode().Perm() != 0o600 {
			t.Fatalf("widened permissions must be narrowed on read: %v", fi.Mode())
		}
	}
	key, src, err := Get("anthropic")
	if err != nil || key != "sk-ant-file-0001" || src != FromFile {
		t.Fatalf("Get = %q %s %v", key, src, err)
	}
	if !strings.Contains(Describe(src), FilePath()) {
		t.Fatalf("the description must name the file: %s", Describe(src))
	}
	if err := Delete("anthropic"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Get("anthropic"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
	if key, _, _ := Get("openai"); key != "sk-openai-0002" {
		t.Fatalf("the other provider's key must stay: %q", key)
	}
	if err := Delete("anthropic"); err == nil {
		t.Fatal("deleting a key that is not there is an error")
	}
	if err := Delete("openai"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(FilePath()); !os.IsNotExist(err) {
		t.Fatalf("an empty key file is removed: %v", err)
	}
}

func TestLinuxSecretService(t *testing.T) {
	f := linux(t, true)
	// A key saved to the file before a keyring existed moves on the next save.
	f.down = true
	if src, err := Set("anthropic", "sk-ant-old-0000"); err != nil || src != FromFile {
		t.Fatalf("a keyring that does not answer falls back to the file: %s %v", src, err)
	}
	if Store() != FromFile {
		t.Fatalf("no keyring answers: Store = %s", Store())
	}
	f.down = false
	if Store() != FromSecretService {
		t.Fatalf("a keyring answers: Store = %s", Store())
	}
	if key, src, _ := Get("anthropic"); key != "sk-ant-old-0000" || src != FromFile {
		t.Fatalf("the file is still read when the keyring has nothing: %q %s", key, src)
	}
	src, err := Set("anthropic", "sk-ant-ring-0001")
	if err != nil || src != FromSecretService {
		t.Fatalf("Set = %s %v", src, err)
	}
	if f.keys["anthropic"] != "sk-ant-ring-0001" {
		t.Fatalf("the key goes to secret-tool on stdin: %v", f.keys)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "sk-ant") {
			t.Fatalf("the key must never be an argument: %s", c)
		}
	}
	if _, err := os.Stat(FilePath()); !os.IsNotExist(err) {
		t.Fatalf("the stale copy in the file must go: %v", err)
	}
	key, src, err := Get("anthropic")
	if err != nil || key != "sk-ant-ring-0001" || src != FromSecretService {
		t.Fatalf("Get = %q %s %v", key, src, err)
	}
	if err := Delete("anthropic"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.keys["anthropic"]; ok {
		t.Fatal("still in the keyring")
	}
}

func TestEnvWinsAndOtherSystems(t *testing.T) {
	linux(t, false)
	if _, err := Set("anthropic", "sk-ant-file-0001"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-env-0009")
	if key, src, _ := Get("anthropic"); key != "sk-ant-env-0009" || src != FromEnv {
		t.Fatalf("the environment wins: %q %s", key, src)
	}
	goos = "windows"
	if _, err := Set("anthropic", "x"); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("no key storage there: the error must point at the variable: %v", err)
	}
}

// A warning on standard error is not part of the key.
func TestKeyIsStandardOutputOnly(t *testing.T) {
	f := linux(t, true)
	f.warning = "(secret-tool:4242): GLib-GObject-WARNING **: 10:00:00.000: invalid cast\n"
	if _, err := Set("anthropic", "sk-ant-ring-0001"); err != nil {
		t.Fatal(err)
	}
	key, src, err := Get("anthropic")
	if err != nil || key != "sk-ant-ring-0001" || src != FromSecretService {
		t.Fatalf("Get = %q %s %v", key, src, err)
	}
}

// Key A in the keyring; then a save with no session bus (over SSH) writes B
// to the file. Back on the desktop the newer key must win, and the next save
// leaves a single copy.
func TestNewerKeyInFileWinsOverStaleKeyring(t *testing.T) {
	f := linux(t, true)
	if src, _ := Set("anthropic", "sk-ant-AAAA-0001"); src != FromSecretService {
		t.Fatalf("first save: %s", src)
	}
	f.down = true
	if src, _ := Set("anthropic", "sk-ant-BBBB-0002"); src != FromFile {
		t.Fatalf("save with no keyring: %s", src)
	}
	if key, src, _ := Get("anthropic"); key != "sk-ant-BBBB-0002" || src != FromFile {
		t.Fatalf("over SSH: %q %s", key, src)
	}
	f.down = false
	if key, src, _ := Get("anthropic"); key != "sk-ant-BBBB-0002" || src != FromFile {
		t.Fatalf("back on the desktop the stale keyring entry must not win: %q %s", key, src)
	}
	if src, _ := Set("anthropic", "sk-ant-CCCC-0003"); src != FromSecretService {
		t.Fatalf("third save: %s", src)
	}
	if key, src, _ := Get("anthropic"); key != "sk-ant-CCCC-0003" || src != FromSecretService {
		t.Fatalf("one copy, in the keyring: %q %s", key, src)
	}
	if len(readFile()) != 0 {
		t.Fatalf("nothing may be left in the file: %v", readFile())
	}
}

// auth remove clears both stores, also when the keyring does not answer at
// that moment: its copy must not come back later.
func TestDeleteClearsBothStores(t *testing.T) {
	f := linux(t, true)
	Set("anthropic", "sk-ant-AAAA-0001")
	f.down = true
	Set("anthropic", "sk-ant-BBBB-0002")
	f.down = false
	if err := Delete("anthropic"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.keys["anthropic"]; ok || len(readFile()) != 0 {
		t.Fatalf("both copies must go: keyring %v, file %v", f.keys, readFile())
	}
	if _, _, err := Get("anthropic"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}

	// Removed over SSH while the keyring still holds a copy.
	Set("openai", "sk-openai-AAAA-0001")
	f.down = true
	if err := Delete("openai"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Get("openai"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed with no keyring answering: %v", err)
	}
	f.down = false
	if _, _, err := Get("openai"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the keyring's copy must not come back: %v", err)
	}
	if _, ok := f.keys["openai"]; ok {
		t.Fatal("the keyring's copy is removed once the keyring answers")
	}
	if _, err := os.Stat(FilePath()); !os.IsNotExist(err) {
		t.Fatalf("the removal mark goes too: %v", err)
	}
	// A new key after a removal mark is found again.
	f.down = true
	Delete("openai")
	f.down = false
	if src, _ := Set("openai", "sk-openai-BBBB-0002"); src != FromSecretService {
		t.Fatalf("save: %s", src)
	}
	if key, _, _ := Get("openai"); key != "sk-openai-BBBB-0002" {
		t.Fatalf("Get after a new save: %q", key)
	}
}
