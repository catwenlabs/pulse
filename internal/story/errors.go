package story

import "errors"

// ErrSelfMerge is returned when a Story is merged into itself.
var ErrSelfMerge = errors.New("cannot merge a story into itself")
