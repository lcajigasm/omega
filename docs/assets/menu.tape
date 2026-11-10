# Records the home screen from the omega binary.
# scripts/capture.sh writes a config for a local provider and then runs this tape.
Output bin/menu.gif
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
Type "omega menu"
Enter
# A heading every system has: the captures are recorded on Linux, where
# the Mac group is not offered.
Wait+Screen@25s /MAINTENANCE/
Sleep 2s
Screenshot docs/assets/menu.png
# The screenshot is the next frame recorded: leave the menu up for it.
Sleep 1500ms
Type "q"
Sleep 500ms
