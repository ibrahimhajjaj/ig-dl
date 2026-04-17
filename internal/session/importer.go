package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

// Import reads and decodes a session.json file produced either by the
// companion browser extension or by a previous Save call. It validates
// that the decoded session carries at least one cookie and a non-zero
// CapturedAt timestamp; anything less is not a usable session.
func Import(path string) (*types.Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session import: read %s: %w", path, err)
	}
	var s types.Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("session import: decode %s: %w", path, err)
	}
	if len(s.Cookies) == 0 {
		return nil, errors.New("session import: validation failed: cookies array is empty")
	}
	if s.CapturedAt.IsZero() {
		return nil, errors.New("session import: validation failed: captured_at is zero")
	}
	return &s, nil
}
