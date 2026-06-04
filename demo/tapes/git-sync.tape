Output demo/assets/git-sync.gif

Source demo/tapes/settings.tape

Hide
Type "cd "
Hide
Type@0ms "@REPO@"
Enter
Type "make demo-prepare"
Enter
Sleep 2s
Show

Type "./bin/remotr --config demo/record/config/config.yaml git sync"
Enter
Sleep 2s
