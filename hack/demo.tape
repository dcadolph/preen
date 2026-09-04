Output assets/demo.gif

Set Shell bash
Set Theme "Catppuccin Mocha"
Set FontSize 15
Set Width 1000
Set Height 1080
Set Padding 25
Set TypingSpeed 55ms

Hide
Type@1ms "PREENREPO=$PWD"
Enter
Type@1ms "source $PREENREPO/hack/demo-env.sh"
Enter
Sleep 4s
Type@1ms "export PS1='$ '"
Enter
Type@1ms "clear"
Enter
Sleep 500ms
Show

Sleep 1s
Type "git status --short"
Enter
Sleep 3s

Type "preen"
Enter
Sleep 6s

Type "drop scratch.txt"
Enter
Sleep 5s

Type "y"
Enter
Sleep 6s

Type "git --no-pager log --oneline"
Enter
Sleep 6s
