package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/steps"
)

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: i18n.T("cmd.doctor"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.OutOrStdout())
		},
	}
}

// runDoctor checks the config, the key's shape, whether the provider answers
// and which catalogue steps are installed. It never sends a chat message.
func runDoctor(w io.Writer) error {
	problems := 0
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(w, i18n.T("doctor.config_bad", err.Error()))
		fmt.Fprint(w, doctorSteps(config.Default()))
		return fmt.Errorf("%s", i18n.T("doctor.failed", 1))
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(w, i18n.T("doctor.config_bad", err.Error()))
		problems++
	} else {
		fmt.Fprintln(w, i18n.T("doctor.config_ok", config.Path()))
	}
	problems += doctorProvider(w, cfg)
	fmt.Fprint(w, doctorSteps(cfg))
	if problems > 0 {
		return fmt.Errorf("%s", i18n.T("doctor.failed", problems))
	}
	return nil
}

func doctorProvider(w io.Writer, cfg *config.Config) int {
	info, ok := llm.Lookup(cfg.Provider)
	if !ok {
		fmt.Fprintln(w, i18n.T("doctor.provider_bad", i18n.T("err.unknown_provider", cfg.Provider, providerNames())))
		return 1
	}
	if info.Local {
		fmt.Fprintln(w, i18n.T("doctor.key_local"))
		base := cfg.BaseURLFor(info.Name)
		if base == "" {
			base = info.BaseURL
		}
		if base == "" {
			fmt.Fprintln(w, i18n.T("doctor.provider_bad", i18n.T("doctor.no_base")))
			return 1
		}
		if probeLocal(base) {
			fmt.Fprintln(w, i18n.T("doctor.provider_ok", info.Label))
			return 0
		}
		fmt.Fprintln(w, i18n.T("doctor.provider_bad", i18n.T("model.unreachable", info.Label, base)))
		return 1
	}

	key, src, err := getKey(info.Name)
	if err != nil {
		fmt.Fprintln(w, i18n.T("doctor.key_missing", info.Label))
		return 1
	}
	if keyLooksWrong(key) {
		fmt.Fprintln(w, i18n.T("doctor.key_bad", info.KeyURL))
		return 1
	}
	fmt.Fprintln(w, i18n.T("doctor.key_ok", string(src)))
	if pre := keyPrefix[info.Name]; pre != "" && !strings.HasPrefix(key, pre) {
		fmt.Fprintln(w, i18n.T("doctor.key_prefix", pre))
	}

	p, err := llm.New(info.Name, llm.Options{APIKey: key, BaseURL: cfg.BaseURLFor(info.Name)})
	if err != nil {
		fmt.Fprintln(w, i18n.T("doctor.provider_bad", err.Error()))
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	switch err := llm.Verify(ctx, p); {
	case err == nil:
		fmt.Fprintln(w, i18n.T("doctor.provider_ok", info.Label))
		return 0
	case errors.Is(err, llm.ErrUnauthorized):
		fmt.Fprintln(w, i18n.T("doctor.key_rejected", info.Label))
		return 1
	default:
		fmt.Fprintln(w, i18n.T("doctor.provider_bad", err.Error()))
		return 1
	}
}

func doctorSteps(cfg *config.Config) string {
	installed, missing := 0, 0
	var b strings.Builder
	for _, s := range steps.All {
		mark := "·"
		if s.Available() {
			mark = "✓"
			installed++
		} else {
			missing++
		}
		line := fmt.Sprintf("  %s %s", mark, s.ID)
		if cfg.Skipped(s.ID) {
			line += " (" + i18n.T("doctor.skipped") + ")"
		}
		fmt.Fprintln(&b, line)
	}
	return i18n.T("doctor.steps", installed, missing) + "\n" + b.String()
}
