//go:build windows

package secrets

import "testing"

// A real round trip through the Windows Credential Manager, on a provider
// name nobody uses.
func TestCredentialManagerRoundTrip(t *testing.T) {
	const provider = "omega-test-roundtrip"
	if !credAvailable() {
		t.Skip("advapi32 is not available")
	}
	t.Cleanup(func() { credDelete(provider) })
	if key, err := credRead(provider); err != nil || key != "" {
		t.Fatalf("before saving: %q %v", key, err)
	}
	const secret = "sk-test-ünïcode-0123456789"
	if err := credWrite(provider, secret); err != nil {
		t.Fatal(err)
	}
	if key, err := credRead(provider); err != nil || key != secret {
		t.Fatalf("read back %q %v", key, err)
	}
	// Saving again replaces the key.
	if err := credWrite(provider, "second"); err != nil {
		t.Fatal(err)
	}
	if key, _ := credRead(provider); key != "second" {
		t.Fatalf("after replacing: %q", key)
	}
	if found, err := credDelete(provider); err != nil || !found {
		t.Fatalf("delete: %v %v", found, err)
	}
	if found, err := credDelete(provider); err != nil || found {
		t.Fatalf("second delete: %v %v", found, err)
	}
	if key, _ := credRead(provider); key != "" {
		t.Fatalf("after deleting: %q", key)
	}
}
