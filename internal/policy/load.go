package policy

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadFile reads a JSON policy document and builds an Engine.
func LoadFile(path string) (*Engine, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse policy %s: %w", path, err)
	}
	return New(cfg)
}
