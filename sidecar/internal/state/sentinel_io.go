package state

import (
	"encoding/json"
	"fmt"
	"os"
)

func readSentinel(path string) (TrampolineSentinel, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return TrampolineSentinel{}, fmt.Errorf("read: %w", err)
	}
	var s TrampolineSentinel
	if err := json.Unmarshal(body, &s); err != nil {
		return TrampolineSentinel{}, fmt.Errorf("unmarshal: %w (body=%q)", err, string(body))
	}
	return s, nil
}
