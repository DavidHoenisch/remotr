Output demo/assets/config-validate.gif

Source demo/tapes/settings.tape

Hide
Type "cd "
Hide
Type@0ms "@REPO@"
Enter
Type "make demo-prepare"
Enter
Sleep 1s
Type "test -d demo/workspace || ./bin/remotr --config demo/record/config/config.yaml init -fleet engineering demo/workspace"
Enter
Sleep 2s
Show

Type "./bin/remotr --config demo/record/config/config.yaml config validate demo/workspace"
Enter
Sleep 3s
