package cli

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/BurntSushi/toml"

	"github.com/lcajigasm/omega/internal/config"
)

func tomlEncode(w io.Writer, v any) error { return toml.NewEncoder(w).Encode(v) }

// configJSON writes the configuration as JSON with the keys of config.toml.
func configJSON(w io.Writer, cfg *config.Config) error {
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(cfg); err != nil {
		return err
	}
	var m map[string]any
	if _, err := toml.Decode(b.String(), &m); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

// unknownConfigKeys are the keys of config.toml omega does not know: a
// typo ("aproval") leaves its setting at the default without a word. It
// reads them from the last config.Load, which is done first.
func unknownConfigKeys() []string {
	if _, err := config.Load(); err != nil {
		return nil
	}
	return config.UnknownKeys()
}
