package probe

import "os"

type EnvReader interface {
	Get(key string) string
}

type OSEnvReader struct{}

func (OSEnvReader) Get(key string) string {
	return os.Getenv(key)
}

type MockEnvReader map[string]string

func (m MockEnvReader) Get(key string) string {
	return m[key]
}
