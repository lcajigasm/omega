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
Wait+Screen@25s /YOUR MAC/
Sleep 2s
Screenshot docs/assets/menu.png
Type "q"
Sleep 500ms
