package systemdctl

import "github.com/DavidHoenisch/remotr/internal/executil"

// DaemonReload runs systemctl daemon-reload so unit files written earlier in the
// same apply pass (for example under /etc/systemd/system) are visible to systemctl.
func DaemonReload(exec executil.Runner) error {
	_, _, err := exec.Run("systemctl", "daemon-reload")
	return err
}
