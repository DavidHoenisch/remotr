package models

import (
	"io"

	"gopkg.in/yaml.v3"
)

// ParseCronState reads YAML crons artifact bytes into CronState.
func ParseCronState(r io.Reader) (CronState, error) {
	var state CronState
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&state); err != nil {
		return CronState{}, err
	}
	return state, nil
}
