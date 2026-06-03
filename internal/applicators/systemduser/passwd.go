package systemduser

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// InteractiveUser is a local account managed by systemdUser resources.
type InteractiveUser struct {
	Username string
	UID      int
}

var passwdPath = "/etc/passwd"

// ListInteractiveUsers returns UID>=1000 accounts from passwd, excluding nobody.
func ListInteractiveUsers() ([]InteractiveUser, error) {
	data, err := os.ReadFile(passwdPath) // #nosec G304 -- fixed system path
	if err != nil {
		return nil, err
	}
	return ListInteractiveUsersFromPasswd(string(data))
}

// ListInteractiveUsersFromPasswd parses passwd content without reading the filesystem.
func ListInteractiveUsersFromPasswd(content string) ([]InteractiveUser, error) {
	var users []InteractiveUser
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, ok, err := parsePasswdLine(line)
		if err != nil {
			return nil, err
		}
		if ok {
			users = append(users, u)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func parsePasswdLine(line string) (InteractiveUser, bool, error) {
	fields := strings.Split(line, ":")
	if len(fields) < 3 {
		return InteractiveUser{}, false, fmt.Errorf("invalid passwd line: %q", line)
	}
	username := fields[0]
	uid, err := strconv.Atoi(fields[2])
	if err != nil {
		return InteractiveUser{}, false, fmt.Errorf("invalid uid in passwd line: %q", line)
	}
	if uid < 1000 || username == "nobody" {
		return InteractiveUser{}, false, nil
	}
	return InteractiveUser{Username: username, UID: uid}, true, nil
}
