package health

import (
	"encoding/json"
	"os"
)

// bootState mirrors the startup marker written by the main application.
type bootState struct {
	Phase     string `json:"phase"`
	Migration string `json:"migration"`
	At        string `json:"at"`
}

func (c *Checker) readBootState() bootState {
	if c.BootStatePath == "" {
		return bootState{}
	}
	data, err := os.ReadFile(c.BootStatePath)
	if err != nil {
		return bootState{}
	}
	var bs bootState
	if err := json.Unmarshal(data, &bs); err != nil {
		return bootState{}
	}
	return bs
}
