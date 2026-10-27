# Records the sudo password card from a real run:  vhs docs/assets/sudo.tape
Output docs/assets/sudo.gif
Set Shell zsh
Set FontSize 15
Set Width 1280
Set Height 820
Set Padding 18
Set Theme "Catppuccin Mocha"
Hide
Type "export OMEGA_LANG=en PATH=$PWD/bin:$PATH PS1='$ ' && clear"
Enter
Show
Type `omega -p 'Check whether this Mac grants admin access: run exactly sudo -n true with run_command, then answer in one sentence.'`
Enter
Wait+Screen@60s /Run this\?/
Sleep 800ms
Type "y"
Wait+Screen@30s /sudo needs your password/
Sleep 1s
Screenshot docs/assets/sudo.png
Escape
Wait+Screen@60s /Ask for something else/
Sleep 1.5s
Screenshot bin/sudo-after.png
Escape
Sleep 1s
Type "q"
Sleep 500ms
