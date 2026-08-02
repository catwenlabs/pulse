package story

import "errors"

// ErrSelfMerge is returned when a Story is merged into itself.
var ErrSelfMerge = errors.New("cannot merge a story into itself")

// ErrMetadataConflict requires the caller to choose the surviving Story
// metadata before a manual merge can commit.
var ErrMetadataConflict = errors.New("story metadata conflict requires resolution")

// ErrReclusterUnavailable is returned when on-demand Story recluster is requested
// but no aggregation processor is wired into the backend.
var ErrReclusterUnavailable = errors.New("story recluster is not available")
