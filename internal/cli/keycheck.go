package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
)

// keyPrefix is the usual prefix of each provider's keys, used only to warn.
var keyPrefix = map[string]string{
	"anthropic":  "sk-ant-",
	"openai":     "sk-",
	"openrouter": "sk-or-",
}

// cleanKey removes what terminals and clipboards wrap around pasted text:
// bracketed-paste markers, quotes and surrounding whitespace.
func cleanKey(k string) string {
	k = strings.ReplaceAll(k, "\x1b[200~", "")
	k = strings.ReplaceAll(k, "\x1b[201~", "")
	k = strings.TrimSpace(k)
	return strings.Trim(k, `"'`)
}

// keyLooksWrong reports keys that cannot be API keys, such as URLs or text
// with spaces.
func keyLooksWrong(k string) bool {
	return strings.Contains(k, "://") || strings.ContainsAny(k, " \t\r\n") || len(k) < 20
}

// checkKey validates a key's shape and asks the provider whether it accepts
// it. Network problems only warn, so an offline setup can still save a key.
func checkKey(cfg *config.Config, provider, key string) error {
	info, _ := llm.Lookup(provider)
	if keyLooksWrong(key) {
		return fmt.Errorf("%s", i18n.T("setup.key_url", info.KeyURL))
	}
	if pre := keyPrefix[provider]; pre != "" && !strings.HasPrefix(key, pre) {
		fmt.Println(i18n.T("setup.key_prefix", info.Label, pre))
	}
	p, err := llm.New(provider, llm.Options{APIKey: key, BaseURL: cfg.BaseURLFor(provider)})
	if err != nil {
		return err
	}
	fmt.Println(i18n.T("setup.key_checking", info.Label))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	switch err := llm.Verify(ctx, p); {
	case err == nil:
		fmt.Println(i18n.T("setup.key_ok"))
	case errors.Is(err, llm.ErrUnauthorized):
		return fmt.Errorf("%s", i18n.T("setup.key_rejected", info.Label, err))
	default:
		fmt.Println(i18n.T("setup.key_unverified", err))
	}
	return nil
}
