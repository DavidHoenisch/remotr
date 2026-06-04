Output demo/assets/bootstrap.gif

Source demo/tapes/settings.tape

Hide
Type "cd "
Hide
Type@0ms "@REPO@"
Enter
Type "make demo-prepare-bootstrap"
Enter
Sleep 2s
Show

Type "./bin/remotr --config demo/record/config/config.yaml bootstrap --token demo-bootstrap-token"
Enter
Sleep 2s

Type "ls demo/record/state/"
Enter
Sleep 3s
