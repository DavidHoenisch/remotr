package models

import (
	"io"

	"gopkg.in/yaml.v3"
)

// ParseState reads YAML deployable artifact bytes into State.
func ParseState(r io.Reader) (State, error) {
	var state State
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&state); err != nil {
		return State{}, err
	}
	return state, nil
}
