package types

// Kind identifies a self-describing configuration file in a config repository.
type Kind string

const (
	KindManifest    Kind = "manifest"
	KindModule      Kind = "module"
	KindApplication Kind = "application"
	KindCrons       Kind = "crons"
)

// Valid reports whether k is a known configuration kind.
func (k Kind) Valid() bool {
	switch k {
	case KindManifest, KindModule, KindApplication, KindCrons:
		return true
	default:
		return false
	}
}

// String returns the YAML kind value.
func (k Kind) String() string {
	return string(k)
}
