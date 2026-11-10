//go:build windows

package ui

// diskSpace is not measured on Windows: the session panel leaves the row out.
func diskSpace(string) (free, total uint64) { return 0, 0 }
