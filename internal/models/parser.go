package models

import (
	"encoding/json"
	"io"
)

// ParseState takes s of type io.Reader and marshals
// into State struct
func ParseState(s io.Reader) (State, error) {
	var state State
	if err := json.NewDecoder(s).Decode(&state); err != nil {
		return State{}, err
	}
	return state, nil
}
