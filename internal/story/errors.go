package story

import "errors"

// ErrSelfMerge is returned when a Story is merged into itself.
var ErrSelfMerge = errors.New("cannot merge a story into itself")

// ErrRecomputeUnavailable is returned when on-demand Story recompute is requested
// but no aggregation processor is wired into the backend.
var ErrRecomputeUnavailable = errors.New("story recompute is not available")
