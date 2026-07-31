package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/catwenlabs/pulse/internal/annotation"
	"github.com/catwenlabs/pulse/internal/source"
)

var ErrDriverNotFound = errors.New("driver not found")

type Trigger string

const (
	TriggerSchedule   Trigger = "schedule"
	TriggerWebhook    Trigger = "webhook"
	TriggerFileChange Trigger = "file-change"
	TriggerManual     Trigger = "manual"
	TriggerImport     Trigger = "import"
)

type Checkpoint json.RawMessage

type AcquireRequest struct {
	Source     source.Source
	Trigger    Trigger
	Payload    io.Reader
	Checkpoint Checkpoint
	Limits     Limits
}

type Limits struct {
	MaxBytes    int64
	MaxEntries  int
	MaxPages    int
	MaxDuration time.Duration
}

type Candidate struct {
	ExternalID  string
	URL         string
	Title       string
	Author      string
	Summary     string
	ContentHTML string
	PublishedAt *time.Time
	RawMeta     map[string]any
	Annotation  *annotation.Detail
}

type Diagnostics struct {
	Status         string
	CandidateCount int
	Details        map[string]string
}

type AcquisitionBatch struct {
	Candidates     []Candidate
	NextCheckpoint Checkpoint
	SuggestedNext  *time.Time
	Diagnostics    Diagnostics
}

type Driver interface {
	Kind() source.Kind
	Validate(context.Context, source.Spec) (source.ValidatedSpec, error)
	Acquire(context.Context, AcquireRequest) (AcquisitionBatch, error)
}

type Registry struct {
	drivers map[source.Kind]Driver
}

func NewRegistry(drivers ...Driver) (*Registry, error) {
	registry := &Registry{drivers: make(map[source.Kind]Driver, len(drivers))}
	for _, driver := range drivers {
		if driver == nil {
			return nil, fmt.Errorf("register driver: nil driver")
		}
		kind := driver.Kind()
		if _, exists := registry.drivers[kind]; exists {
			return nil, fmt.Errorf("register driver %q: duplicate kind", kind)
		}
		registry.drivers[kind] = driver
	}
	return registry, nil
}

func (registry *Registry) Driver(kind source.Kind) (Driver, error) {
	driver, ok := registry.drivers[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDriverNotFound, kind)
	}
	return driver, nil
}
