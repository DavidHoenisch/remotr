package systemduser

import "github.com/DavidHoenisch/remotr/internal/interactiveuser"

// InteractiveUser is a local account managed by systemdUser resources.
type InteractiveUser = interactiveuser.Account

// ListInteractiveUsers returns UID>=interactiveuser.MinUID accounts from passwd, excluding nobody.
func ListInteractiveUsers() ([]InteractiveUser, error) {
	return interactiveuser.List()
}

// ListInteractiveUsersFromPasswd parses passwd content without reading the filesystem.
func ListInteractiveUsersFromPasswd(content string) ([]InteractiveUser, error) {
	return interactiveuser.ParsePasswd(content)
}
