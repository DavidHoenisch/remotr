Output demo/assets/init.gif

Source demo/tapes/settings.tape

Hide
Type "cd "
Hide
Type@0ms "@REPO@"
Enter
Type "make demo-prepare"
Enter
Sleep 2s
Type "rm -rf demo/workspace && mkdir -p demo/workspace"
Enter
Sleep 500ms
Show

Type "./bin/remotr --config demo/record/config/config.yaml init -fleet engineering demo/workspace"
Enter
Sleep 2s

Type "ls -la demo/workspace/fleets/engineering/"
Enter
Sleep 3s
