package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidCursor = errors.New("invalid pagination cursor")

// Position is the opaque position encoded in a list cursor. Filter values are
// part of the cursor so a cursor cannot accidentally be reused with a
// different list query.
type Position struct {
	Kind     string    `json:"kind"`
	Search   string    `json:"search"`
	State    string    `json:"state"`
	Tag      string    `json:"tag"`
	SourceID string    `json:"source_id"`
	Bucket   int       `json:"bucket,omitempty"`
	Time     time.Time `json:"time"`
	ID       string    `json:"id"`
}

func Encode(position Position) (string, error) {
	if position.Kind == "" || position.ID == "" || position.Time.IsZero() {
		return "", fmt.Errorf("%w: missing position fields", ErrInvalidCursor)
	}
	payload, err := json.Marshal(position)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func Decode(
	raw string,
	kind string,
	search string,
	state string,
	tag string,
	sourceID string,
) (Position, error) {
	if strings.TrimSpace(raw) == "" {
		return Position{}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Position{}, fmt.Errorf("%w: malformed encoding", ErrInvalidCursor)
	}
	var position Position
	if err := json.Unmarshal(data, &position); err != nil {
		return Position{}, fmt.Errorf("%w: malformed payload", ErrInvalidCursor)
	}
	if position.Kind != kind ||
		position.Search != search ||
		position.State != state ||
		position.Tag != tag ||
		position.SourceID != sourceID ||
		!validUUID(position.ID) ||
		position.Time.IsZero() {
		return Position{}, fmt.Errorf("%w: cursor does not match the active query", ErrInvalidCursor)
	}
	if position.Bucket < 0 || position.Bucket > 1 {
		return Position{}, fmt.Errorf("%w: invalid sort bucket", ErrInvalidCursor)
	}
	if kind == "source_entries" && position.Bucket != 0 {
		return Position{}, fmt.Errorf("%w: invalid Source Entry sort bucket", ErrInvalidCursor)
	}
	return position, nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' ||
		value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !((character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'f') ||
			(character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}
