# Records the README screenshots and GIF from omega's scripted demo session.
# Run from the repository root:  make build && vhs docs/assets/demo.tape
Output docs/assets/demo.gif

Set Shell zsh
Set FontSize 15
Set Width 1280
Set Height 820
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms

Hide
Type "export OMEGA_LANG=en PATH=$PWD/bin:$PATH PS1='$ ' && clear"
Enter
Show

Type "omega --demo"
Sleep 400ms
Enter

Wait+Screen@40s /running/
Sleep 1.2s
Screenshot docs/assets/running.png

Wait+Screen@60s /Run this\?/
Sleep 800ms
Screenshot docs/assets/confirm.png
Sleep 1.5s
Type "y"

Wait+Screen@60s /What should I clean up\?/
Sleep 1s
Space
Sleep 400ms
Down
Sleep 300ms
Space
Sleep 400ms
Down
Sleep 300ms
Down
Sleep 300ms
Space
Sleep 800ms
Screenshot docs/assets/card.png
Sleep 1s
Enter

Wait+Screen@30s /Run this\?/
Sleep 1s
Type "y"
Wait+Screen@30s /always need your confirmation/
Sleep 800ms
Screenshot docs/assets/docker-confirm.png
Sleep 1.2s
Type "y"
Wait+Screen@30s /Run this\?/
Sleep 1s
Type "y"

Wait+Screen@60s /All done/
Sleep 2.5s
Screenshot docs/assets/summary.png
Sleep 2s
Escape
Sleep 1s
Type "q"
Sleep 500ms
