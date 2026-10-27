package cli

import (
	"os"

	"github.com/BurntSushi/toml"
)

func tomlEncode(v any) error { return toml.NewEncoder(os.Stdout).Encode(v) }
