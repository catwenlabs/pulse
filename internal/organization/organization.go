package organization

import "github.com/wenpengfei/pulse/internal/entry"

type Folder struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SourceCount int    `json:"source_count"`
}

type View struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Query entry.Query `json:"query"`
}
