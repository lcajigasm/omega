// Package sudo lets commands run by omega use sudo without touching the
// terminal. Commands run in their own session with no controlling terminal,
// a small `sudo` wrapper is put first on their PATH, and it asks for the
// password through SUDO_ASKPASS, which reaches omega over a private Unix
// socket. omega asks the user in its own interface, checks the password with
// sudo and keeps it in memory for the rest of the run only.
package sudo

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// AskFunc asks the user for the password. retry is true when the previous
// attempt was wrong. ok is false when the user cancels.
type AskFunc func(prompt string, retry bool) (password string, ok bool)

// Bridge serves password requests for one omega run.
type Bridge struct {
	dir      string
	ln       net.Listener
	ask      AskFunc
	mu       sync.Mutex
	password string
	refused  bool
}

const helperArg = "__askpass"

// wrapper strips the options that would stop sudo from asking (-n, -S) from
// sudo's own flags only, then forces the askpass helper with -A.
const wrapper = `#!/bin/bash
# omega: route sudo's password prompt to omega's interface.
opts=()
while [[ $# -gt 0 && "$1" == -* && "$1" != "--" ]]; do
  case "$1" in
    -n|--non-interactive|-S|--stdin) ;;
    -u|-g|-p|-C|-D|-h|-r|-t|-T|-U) opts+=("$1" "$2"); shift ;;
    *) opts+=("$1") ;;
  esac
  shift
done
exec /usr/bin/sudo -A "${opts[@]}" "$@"
`

// Start creates the wrapper, the askpass helper and the socket.
func Start(ask AskFunc, password string) (*Bridge, error) {
	dir, err := os.MkdirTemp("", "omega-sudo-")
	if err != nil {
		return nil, err
	}
	b := &Bridge{dir: dir, ask: ask, password: password}
	sock := filepath.Join(dir, "askpass.sock")
	if b.ln, err = net.Listen("unix", sock); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		b.Close()
		return nil, err
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		b.Close()
		return nil, err
	}
	helper := fmt.Sprintf("#!/bin/sh\nOMEGA_ASKPASS_SOCK=%s exec %s %s \"$1\"\n", shellQuote(sock), shellQuote(exe), helperArg)
	if err := os.WriteFile(filepath.Join(dir, "askpass"), []byte(helper), 0o700); err != nil {
		b.Close()
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte(wrapper), 0o700); err != nil {
		b.Close()
		return nil, err
	}
	go b.serve()
	return b, nil
}

// Env is the environment commands need to use the bridge.
func (b *Bridge) Env() []string {
	return []string{"SUDO_ASKPASS=" + filepath.Join(b.dir, "askpass")}
}

// PathPrefix is the directory that must come first on the commands' PATH.
func (b *Bridge) PathPrefix() string { return filepath.Join(b.dir, "bin") }

// Close stops the bridge and removes its files.
func (b *Bridge) Close() {
	if b.ln != nil {
		b.ln.Close()
	}
	os.RemoveAll(b.dir)
}

func (b *Bridge) serve() {
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}
		go b.handle(conn)
	}
}

func (b *Bridge) handle(conn net.Conn) {
	defer conn.Close()
	prompt, _ := bufio.NewReader(conn).ReadString('\n')
	prompt = strings.TrimSpace(prompt)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.password == "" && !b.refused {
		for attempt := 0; attempt < 3; attempt++ {
			pw, ok := b.ask(prompt, attempt > 0)
			if !ok {
				break
			}
			if Validate(pw) {
				b.password = pw
				break
			}
		}
		// Without a valid password, do not ask again in this run: tools probe
		// sudo repeatedly and the user already said no.
		b.refused = b.password == ""
	}
	if b.password != "" {
		fmt.Fprintln(conn, b.password)
	}
}

// Validate checks a password with sudo itself, ignoring any cached ticket.
func Validate(password string) bool {
	cmd := exec.Command("/usr/bin/sudo", "-S", "-k", "-v", "-p", "")
	cmd.Stdin = strings.NewReader(password + "\n")
	setSession(cmd)
	return cmd.Run() == nil
}

// IsHelper reports whether the process was started as the askpass helper.
func IsHelper(args []string) bool { return len(args) > 1 && args[1] == helperArg }

// RunHelper is the askpass program: it forwards the prompt to omega and
// prints the password sudo should use. It exits non-zero when there is none.
func RunHelper(args []string) int {
	conn, err := net.Dial("unix", os.Getenv("OMEGA_ASKPASS_SOCK"))
	if err != nil {
		return 1
	}
	defer conn.Close()
	prompt := "Password:"
	if len(args) > 2 {
		prompt = args[2]
	}
	fmt.Fprintln(conn, prompt)
	pw, _ := bufio.NewReader(conn).ReadString('\n')
	pw = strings.TrimRight(pw, "\n")
	if pw == "" {
		return 1
	}
	fmt.Println(pw)
	return 0
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
