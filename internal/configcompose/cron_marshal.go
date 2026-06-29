package configcompose

import (
	"bytes"

	"github.com/DavidHoenisch/remotr/internal/models"
	"gopkg.in/yaml.v3"
)

func marshalCronState(state models.CronState) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(state); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
