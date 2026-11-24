package safety

import "testing"

// A4 of the fifth review: packing a folder of keys prints them as surely
// as cat does, so an archiver given one is a reader like any other.
func TestArchiversOfSecretsAreBlocked(t *testing.T) {
	for _, cmd := range []string{
		`tar -cf - ~/.ssh | base64`, `tar czf - ~/.ssh`, `tar -C ~ -czf - .ssh`, `bsdtar -cf - ~/.gnupg`,
		`zip -r - ~/.ssh`, `7z a /tmp/k.7z ~/.ssh`, `find ~/.ssh | cpio -o`, `pax -w ~/.ssh`,
		`ditto -c -k ~/.ssh /tmp/k.zip`, `rsync -a ~/.ssh/ /tmp/k/`, `tar -cf /tmp/k.tar ~/.aws`,
		`cd ~ && tar -cf - .ssh`,
	} {
		if got := Classify(cmd); got != Blocked {
			t.Errorf("Classify(%q) = %v, want blocked", cmd, got)
		}
	}
	for _, cmd := range []string{
		`tar -czf /tmp/dev.tgz ~/Dev/app`, `tar -xzf x.tgz -C ~/Downloads`, `tar -tf x.tar`,
		`zip -r /tmp/a.zip ~/Dev/app`, `rsync -a ~/Dev/app/ /Volumes/b/app/`, `7z l x.7z`,
	} {
		if got := Classify(cmd); got == Blocked {
			t.Errorf("Classify(%q) = blocked, want it allowed with approval", cmd)
		}
	}
}
