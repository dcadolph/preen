Output assets/demo.gif

Set Shell zsh
Set Theme "iTerm2 Default"
Set FontFamily "Menlo"
Set FontSize 18
Set Width 1100
Set Height 760
Set Padding 28
Set TypingSpeed 75ms

Hide
Type@1ms "source hack/demo-env.sh"
Enter
Sleep 1500ms
Type@1ms "PROMPT='%F{green}➜%f  %F{cyan}~/demo%f '"
Enter
Type@1ms "clear"
Enter
Sleep 500ms
Show

Type "git status"
Enter
Sleep 3s

Type 'git add -A && git commit -m "wip"'
Enter
Sleep 2500ms

Type "gus"
Enter
Sleep 4s

Type "git log --oneline"
Enter
Sleep 4s
