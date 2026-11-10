package cli

import (
	"regexp"

	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/sudo"
)

var usesSudo = regexp.MustCompile(`(^|[\s;&|(])sudo\s`)

// sudoSession lets commands run from the command line use sudo: they run
// without a terminal, so the password is asked here, once, the first time a
// command needs it. A machine where sudo needs no password is never asked.
type sudoSession struct{ bridge *sudo.Bridge }

// need prepares sudo when the command uses it.
func (s *sudoSession) need(command string) error {
	if s.bridge != nil || !usesSudo.MatchString(command) {
		return nil
	}
	b, err := sudo.Start(askTerminalPassword, "")
	if err != nil {
		return err
	}
	s.bridge = b
	runner.Configure(b.Env(), b.PathPrefix())
	return nil
}

func (s *sudoSession) close() {
	if s.bridge != nil {
		s.bridge.Close()
		runner.Configure(nil, "")
		s.bridge = nil
	}
}
