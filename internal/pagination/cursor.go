package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const currentVersion = 1

type Envelope struct {
	Version int      `json:"v"`
	Keys    []string `json:"k"`
}

var ErrInvalidCursor = fmt.Errorf("invalid cursor")

func Encode(values ...string) (string, error) {
	env := Envelope{Version: currentVersion, Keys: values}
	data, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(data), nil
}

func Decode(raw string, expected int) ([]string, error) {
	if raw == "" {
		if expected != 0 {
			return nil, fmt.Errorf("%w: expected %d values, got 0", ErrInvalidCursor, expected)
		}
		return nil, nil
	}

	data, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}

	if env.Version != currentVersion {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrInvalidCursor, env.Version)
	}

	if len(env.Keys) != expected {
		return nil, fmt.Errorf("%w: expected %d values, got %d", ErrInvalidCursor, expected, len(env.Keys))
	}

	return env.Keys, nil
}
