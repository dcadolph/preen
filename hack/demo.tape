Output assets/demo.gif

Set Shell zsh
Set Theme "iTerm2 Default"
Set FontFamily "Menlo"
Set FontSize 18
Set Width 1100
Set Height 760
Set Padding 28
Set TypingSpeed 75ms

# Scenario one: split a messy working tree.
Hide
Type@1ms "PREENREPO=$PWD"
Enter
Type@1ms "source $PREENREPO/hack/demo-env.sh"
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

Type "preen"
Enter
Sleep 4s

Type "git log --oneline"
Enter
Sleep 3500ms

# Scenario two: commit and push a mess, then rewrite the pushed history.
Hide
Type@1ms "source $PREENREPO/hack/demo-env-pushed.sh"
Enter
Sleep 1500ms
Type@1ms "clear"
Enter
Sleep 500ms
Show

Type 'git add -A && git commit -m "working now"'
Enter
Sleep 2200ms

Type "git push"
Enter
Sleep 2800ms

Type "git log --oneline"
Enter
Sleep 3s

Type "preen"
Enter
Sleep 4s

Type "git log --oneline"
Enter
Sleep 4s

# Scenario three: fold review fixes into the commits that introduced them.
Hide
Type@1ms "source $PREENREPO/hack/demo-env-fixup.sh"
Enter
Sleep 1500ms
Type@1ms "clear"
Enter
Sleep 500ms
Show

Type "git log --oneline"
Enter
Sleep 2500ms

Type "git status"
Enter
Sleep 3s

Type "preen --fixup"
Enter
Sleep 4s

Type "git log --oneline"
Enter
Sleep 4s
