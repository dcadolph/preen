Output assets/agent-demo.gif

Set Shell bash
Set Theme "Catppuccin Mocha"
Set FontSize 15
Set Width 1240
Set Height 640
Set Padding 25
Set TypingSpeed 45ms

Hide
Type@1ms "PREENREPO=$PWD"
Enter
Type@1ms "source $PREENREPO/hack/agent-demo-env.sh"
Enter
Sleep 6s
Type@1ms "export PS1='$ '"
Enter
Type@1ms "clear"
Enter
Sleep 500ms
Show

Sleep 1s
Type "# Two unrelated changes, both in client.go, plus a scratch note."
Enter
Type "git status --short"
Enter
Sleep 3s

Type "preen request > /tmp/preen-request.json"
Enter
Sleep 1s
Type "# The agent answers from what it changed and why."
Enter
Type "cat /tmp/preen-answer.json"
Enter
Sleep 6s

Type "clear"
Enter
Type "preen --grouping /tmp/preen-answer.json --leave notes.md"
Enter
Sleep 5s
Type "y"
Enter
Sleep 5s
Type "git --no-pager log --oneline"
Enter
Sleep 5s

Type "clear"
Enter
Type "# Undo, then change the tree after the answer was written."
Enter
Type "preen restore --yes"
Enter
Sleep 3s
Type "echo '// one more tweak' >> client/client.go"
Enter
Type "preen --grouping /tmp/preen-answer.json --leave notes.md"
Enter
Sleep 5s
Type "# Nothing was committed."
Enter
Type "git --no-pager log --oneline"
Enter
Sleep 8s
