# Records the model picker screenshot:  vhs docs/assets/model.tape
Output docs/assets/model.gif
Set Shell zsh
Set FontSize 15
Set Width 1280
Set Height 560
Set Padding 18
Set Theme "Catppuccin Mocha"
Hide
Type "export OMEGA_LANG=en PATH=$PWD/bin:$PATH PS1='$ ' && clear"
Enter
Show
Type "omega model"
Enter
Wait+Screen@20s /Automatic/
Sleep 1s
Down
Sleep 600ms
Screenshot docs/assets/model.png
Escape
Sleep 500ms
