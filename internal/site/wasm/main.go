//go:build js && wasm

// Command wasm is omega's risk classifier for the website: the same
// safety.Classify the agent calls before every command, compiled to
// WebAssembly so the page can classify what a visitor types.
package main

import (
	"syscall/js"

	"github.com/lcajigasm/omega/internal/safety"
)

func main() {
	js.Global().Set("omegaClassify", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		cmd := args[0].String()
		return map[string]any{
			"risk":   safety.Classify(cmd).String(),
			"always": safety.AlwaysConfirm(cmd),
		}
	}))
	select {}
}
