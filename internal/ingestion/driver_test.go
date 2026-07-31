package ingestion

import (
	"context"
	"errors"
	"testing"

	"github.com/catwenlabs/pulse/internal/source"
)

type stubDriver struct {
	kind source.Kind
}

func (driver stubDriver) Kind() source.Kind {
	return driver.kind
}

func (driver stubDriver) Validate(_ context.Context, spec source.Spec) (source.ValidatedSpec, error) {
	return spec.Validate()
}

func (driver stubDriver) Acquire(context.Context, AcquireRequest) (AcquisitionBatch, error) {
	return AcquisitionBatch{}, nil
}

func TestRegistryResolvesRegisteredDriver(t *testing.T) {
	registry, err := NewRegistry(stubDriver{kind: source.KindRSS})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	driver, err := registry.Driver(source.KindRSS)
	if err != nil {
		t.Fatalf("Driver() error = %v", err)
	}
	if driver.Kind() != source.KindRSS {
		t.Errorf("driver kind = %q", driver.Kind())
	}
}

func TestRegistryRejectsDuplicateDriver(t *testing.T) {
	_, err := NewRegistry(
		stubDriver{kind: source.KindRSS},
		stubDriver{kind: source.KindRSS},
	)
	if err == nil {
		t.Fatal("NewRegistry() error = nil, want duplicate error")
	}
}

func TestRegistryReturnsTypedUnknownDriverError(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	_, err = registry.Driver(source.KindRSS)
	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf("Driver() error = %v, want ErrDriverNotFound", err)
	}
}
