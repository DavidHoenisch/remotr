package userutil

import (
	"fmt"
	"os/exec"
)

// Useradd runs useradd for a validated username.
func Useradd(username string) error {
	if err := ValidateLinuxUsername(username); err != nil {
		return err
	}
	cmd := exec.Command("useradd", "--", username) // #nosec G204 -- username validated; -- blocks flags
	return cmd.Run()
}

func UseraddUID(username string, uid int) error {
	if err := ValidateLinuxUsername(username); err != nil {
		return err
	}
	if uid <= 0 {
		return fmt.Errorf("uid must be positive")
	}
	return exec.Command("useradd", "--uid", fmt.Sprintf("%d", uid), "--", username).Run() // #nosec G204 -- validated fixed argv
}

func UsermodUID(username string, uid int) error {
	if err := ValidateLinuxUsername(username); err != nil {
		return err
	}
	if uid <= 0 {
		return fmt.Errorf("uid must be positive")
	}
	return exec.Command("usermod", "--uid", fmt.Sprintf("%d", uid), "--", username).Run() // #nosec G204 -- validated fixed argv
}

// Userdel runs userdel for a validated username.
func Userdel(username string) error {
	if err := ValidateLinuxUsername(username); err != nil {
		return err
	}
	cmd := exec.Command("userdel", "--", username) // #nosec G204 -- username validated; -- blocks flags
	return cmd.Run()
}

// RunUserCommand runs a fixed binary with a validated username argument.
func RunUserCommand(binary string, username string) error {
	if err := ValidateLinuxUsername(username); err != nil {
		return err
	}
	switch binary {
	case "useradd", "userdel":
	default:
		return fmt.Errorf("unsupported binary %q", binary)
	}
	cmd := exec.Command(binary, "--", username) // #nosec G204 -- binary allowlisted; username validated
	return cmd.Run()
}
