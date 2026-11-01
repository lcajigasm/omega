# Records the sudo password card from the omega binary.
# scripts/capture.sh points omega at a local mock model and makes sudo ask
# for a password, then restores passwordless sudo before it exits.
Output bin/sudo.gif
Set Shell bash
Set FontFamily "DejaVu Sans Mono"
Set FontSize 15
Set Width 1280
Set Height 820
Set Padding 18
Set Theme "Catppuccin Mocha"
Hide
Type "export OMEGA_LANG=en PATH=$PWD/bin:$PATH PS1='$ ' && clear"
Enter
Show
Type "omega -p 'Check whether admin access is available.'"
Enter
Wait+Screen@40s /Run this\?/
Sleep 600ms
Type "y"
Wait+Screen@25s /sudo needs your password/
Screenshot docs/assets/sudo.png
Sleep 1.2s
Escape
Sleep 400ms
Ctrl+C
Sleep 300ms
