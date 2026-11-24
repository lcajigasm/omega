package safety

import "testing"

// A1 and A2 of the fifth review: deleting or overwriting the user's own
// files is asked about every time, also with a relative path after a cd or
// in a folder the command does not name, and git's ways of losing work.
func TestAlwaysConfirmDeletesInHome(t *testing.T) {
	for cmd, want := range map[string]bool{
		// A1: relative paths, cd, subshells, xargs.
		`cd ~ && rm -rf Documents`:                   true,
		`cd ~/Documents && rm -rf *`:                 true,
		`(cd ~/Documents && rm -rf x)`:               true,
		`rm -rf node_modules`:                        true,
		`rm -rf ./*`:                                 true,
		`echo ~/Documents | xargs rm -rf`:            true,
		`find . -name '*.log' -print0 | xargs -0 rm`: true,
		`cd ~/Documents && find . -delete`:           true,
		`git clean -fdx`:                             true,
		`cd /tmp && rm -rf build`:                    false,
		`cd ~/Library/Caches && rm -rf *`:            false,
		`cd /tmp; cd - && rm -rf x`:                  true,
		// A2: one file, find -delete, rsync --delete, truncation, shred.
		`rm -f ~/Documents/*.pdf`:                              true,
		`rm -f ~/.zshrc`:                                       true,
		`unlink ~/notes.txt`:                                   true,
		`find ~/Documents -name '*.pdf' -delete`:               true,
		`find ~/Documents -name '*.pdf' -exec rm {} \;`:        true,
		`rsync -a --delete /tmp/empty/ ~/Documents/`:           true,
		`rsync -a --remove-source-files ~/Photos/ /Volumes/x/`: true,
		`rsync -a --delete ~/Dev/ /Volumes/backup/dev/`:        false,
		`: > ~/Documents/thesis.tex`:                           true,
		`echo x >| ~/Documents/thesis.tex`:                     true,
		`echo x >> ~/Documents/log.txt`:                        false,
		`ls > /tmp/out.txt 2>&1`:                               false,
		`truncate -s0 ~/Documents/thesis.tex`:                  true,
		`truncate -s 0 /tmp/x`:                                 false,
		`shred -u ~/Documents/secret.txt`:                      true,
		`echo x | tee ~/Documents/a.txt`:                       true,
		`echo x | tee -a ~/Documents/a.txt`:                    false,
		// The caches and the Trash come back by themselves.
		`rm -rf ~/Library/Caches/com.apple.x`:           false,
		`find ~/.cache/thumbnails -mindepth 1 -delete`:  false,
		`find ~/.local/share/Trash -mindepth 2 -delete`: false,
		`rm -rf ~/.Trash/*`:                             false,
		`rm -f "$d/topgrade.toml"`:                      false,
		`rm -rf /tmp/omega-x`:                           false,
		// git: work that cannot be fetched again.
		`git reset --hard`:              true,
		`git reset --hard origin/main`:  true,
		`git reset HEAD~1`:              false,
		`git stash clear`:               true,
		`git stash drop`:                true,
		`git stash list`:                false,
		`git push --force`:              true,
		`git push --force-with-lease`:   true,
		`git push -f origin main`:       true,
		`git push origin main`:          false,
		`git checkout -- .`:             true,
		`git checkout .`:                true,
		`git checkout main`:             false,
		`git checkout -b feature`:       false,
		`git restore src/x.go`:          true,
		`git restore --staged src/x.go`: false,
		`git branch -D old`:             true,
		`git branch -d old`:             false,
		`git -C ~/Dev/app clean -n`:     false,
	} {
		if got := AlwaysConfirm(cmd); got != want {
			t.Errorf("AlwaysConfirm(%q) = %v, want %v", cmd, got, want)
		}
	}
}

// A1: the deletions it asks about are not blocked: only asked every time.
func TestDeletesInHomeStayDangerous(t *testing.T) {
	for _, cmd := range []string{`rm -rf node_modules`, `cd ~/Documents && rm -rf x`, `rm -f ~/.zshrc`, `git reset --hard`} {
		if got := Classify(cmd); got != Dangerous {
			t.Errorf("Classify(%q) = %v, want dangerous", cmd, got)
		}
	}
}
