//go:build !darwin && !linux

package claudecode

func snapshot() (snap, error) { return snap{}, ErrUnsupported }

func cwdOf(int) string { return "" }

func commandOf(int) string { return "" }
