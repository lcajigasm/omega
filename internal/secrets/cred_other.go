//go:build !windows

package secrets

import "errors"

// The Windows Credential Manager only exists on Windows. These stand in
// elsewhere so the package builds; the tests replace the three seams.

var errNoCredentialManager = errors.New("the Windows Credential Manager is not available on this system")

func credAvailable() bool { return false }

func credWrite(provider, key string) error { return errNoCredentialManager }

func credRead(provider string) (string, error) { return "", errNoCredentialManager }

func credDelete(provider string) (bool, error) { return false, errNoCredentialManager }
