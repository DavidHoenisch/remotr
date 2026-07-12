package rbac

import "testing"

func FuzzAllowIsMonotonicForRuleGroups(f *testing.F) {
	f.Add("GET", "/v1/admin/endpoints", "GET", "/v1/admin/*", "POST", "/v1/admin/enroll-tokens")

	f.Fuzz(func(t *testing.T, method, path, ruleMethod, rulePattern, extraMethod, extraPattern string) {
		if len(method)+len(path)+len(ruleMethod)+len(rulePattern)+len(extraMethod)+len(extraPattern) > 2048 {
			return
		}
		base := []Rule{{Method: ruleMethod, PathPattern: rulePattern}}
		expanded := append(append([]Rule(nil), base...), Rule{Method: extraMethod, PathPattern: extraPattern})
		if Allow(base, method, path) && !Allow(expanded, method, path) {
			t.Fatal("adding a rule revoked existing authorization")
		}
	})
}
