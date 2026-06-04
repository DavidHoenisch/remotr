Output demo/assets/deployment.gif

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

Type "./bin/remotr --config demo/record/config/config.yaml deployment create --label prod-laptops-2026 --ttl 8760h"
Enter
Sleep 2s

Type "./bin/remotr --config demo/record/config/config.yaml deployment list"
Enter
Sleep 2s

Type "./bin/remotr --config demo/record/config/config.yaml deployment show prod-laptops-2026"
Enter
Sleep 3s
