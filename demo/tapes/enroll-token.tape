Output demo/assets/enroll-token.gif

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

Type "./bin/remotr --config demo/record/config/config.yaml enroll token create --ttl 168h"
Enter
Sleep 3s
