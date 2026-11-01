# Records the model picker from the omega binary against a local test server.
# scripts/capture.sh serves GET /v1/models on 127.0.0.1:18080 and then runs this tape.
Output bin/model.gif
Set Shell bash
Set FontFamily "DejaVu Sans Mono"
Set FontSize 15
Set Width 1280
Set Height 560
Set Padding 18
Set Theme "Catppuccin Mocha"
Hide
Type "export OMEGA_LANG=en PATH=$PWD/bin:$PATH PS1='$ ' && clear"
Enter
Show
Type "omega model --provider ollama"
Enter
Wait+Screen@20s /omega-local-test/
Sleep 1s
Screenshot docs/assets/model.png
Escape
Sleep 500ms
