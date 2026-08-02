package organization

import "github.com/catwenlabs/pulse/internal/entry"

type OrderValidationError struct {
	Field   string
	Message string
}

func (err *OrderValidationError) Error() string {
	return err.Message
}

type Folder struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	SourceCount int      `json:"source_count"`
	SourceIDs   []string `json:"source_ids"`
}

type View struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Query entry.Query `json:"query"`
}
