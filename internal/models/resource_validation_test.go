package models

import "testing"

func TestDownloadResourceValidateReloadExecCompatibilityBoundary(t *testing.T) {
	tests := []struct {
		name       string
		reloadExec []string
		wantErr    bool
	}{
		{name: "omitted"},
		{name: "daemon reload", reloadExec: []string{"systemctl", "daemon-reload"}},
		{name: "reload service", reloadExec: []string{"systemctl", "reload", "auditd.service"}},
		{name: "try restart service", reloadExec: []string{"systemctl", "try-restart", "auditd.service"}},
		{name: "restart service", reloadExec: []string{"systemctl", "restart", "auditd.service"}},
		{name: "arbitrary executable", reloadExec: []string{"augenrules", "--load"}, wantErr: true},
		{name: "shell command", reloadExec: []string{"sh", "-c", "systemctl reload auditd"}, wantErr: true},
		{name: "missing service", reloadExec: []string{"systemctl", "reload"}, wantErr: true},
		{name: "empty service", reloadExec: []string{"systemctl", "reload", ""}, wantErr: true},
		{name: "whitespace service", reloadExec: []string{"systemctl", "reload", " auditd.service"}, wantErr: true},
		{name: "unsupported verb", reloadExec: []string{"systemctl", "stop", "auditd.service"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := DownloadResource{
				Name: "audit-rules", URL: "https://example.test/audit.rules", Dest: "/etc/audit/rules.d/audit.rules",
				ReloadExec: test.reloadExec,
			}
			err := resource.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func FuzzDownloadResourceValidateRejectsArbitraryReloadExecutable(f *testing.F) {
	f.Add("augenrules", "--load")
	f.Add("sh", "-c")
	f.Add("service", "auditd")

	f.Fuzz(func(t *testing.T, executable, argument string) {
		if len(executable) > 256 || len(argument) > 256 || executable == "systemctl" {
			// test-exception: EXC-040
			t.Skip()
		}
		resource := DownloadResource{
			Name: "audit-rules", URL: "https://example.test/audit.rules", Dest: "/etc/audit/rules.d/audit.rules",
			ReloadExec: []string{executable, argument},
		}
		if err := resource.Validate(); err == nil {
			t.Fatalf("Validate() accepted reloadExec with arbitrary executable %q", executable)
		}
	})
}
