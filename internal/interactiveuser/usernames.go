package interactiveuser

import (
	"sort"
	"strings"
)

// ListUsernames returns interactive account names sorted by UID ascending.
func ListUsernames() ([]string, error) {
	users, err := List()
	if err != nil {
		return nil, err
	}
	return UsernamesFromAccounts(users), nil
}

// UsernamesFromAccounts extracts usernames sorted by UID ascending.
func UsernamesFromAccounts(users []Account) []string {
	if len(users) == 0 {
		return nil
	}
	sorted := append([]Account(nil), users...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].UID != sorted[j].UID {
			return sorted[i].UID < sorted[j].UID
		}
		return sorted[i].Username < sorted[j].Username
	})
	out := make([]string, 0, len(sorted))
	for _, u := range sorted {
		out = append(out, u.Username)
	}
	return out
}

// JoinUsernames formats usernames for storage or display.
func JoinUsernames(usernames []string) string {
	if len(usernames) == 0 {
		return ""
	}
	return strings.Join(usernames, ",")
}

// SplitUsernames parses comma-separated usernames from storage.
func SplitUsernames(stored string) []string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return nil
	}
	parts := strings.Split(stored, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
