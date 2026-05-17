package types

// defines the method of configuration that
// will be used to bring the OS into the desired
// state
type ConfigMethod int

const (
	// Configures by installing or
	// uninstalling a package
	ManagePackage ConfigMethod = iota + 1

	// Configures by updating a file
	// like /etc/ssh/sshd_config.d
	ManageFile
)
