package story

import "errors"

// ErrSelfMerge is returned when a Story is merged into itself.
var ErrSelfMerge = errors.New("cannot merge a story into itself")

// ErrReclusterUnavailable is returned when on-demand Story recluster is requested
// but no aggregation processor is wired into the backend.
var ErrReclusterUnavailable = errors.New("story recluster is not available")
