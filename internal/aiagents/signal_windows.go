package aiagents

// Scan returns ErrUnsupported on Windows, so nothing is ever ended here.

func terminate(int) {}

func forceKill(int) {}

func alive(int) bool { return false }
