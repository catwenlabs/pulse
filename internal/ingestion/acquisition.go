package ingestion

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/wenpengfei/pulse/internal/source"
)

var ErrNoAcquisition = errors.New("no acquisition available")

type AcquisitionID string

type AcquisitionStatus string

const (
	StatusPending   AcquisitionStatus = "pending"
	StatusRunning   AcquisitionStatus = "running"
	StatusSucceeded AcquisitionStatus = "succeeded"
	StatusRetry     AcquisitionStatus = "retry"
	StatusDead      AcquisitionStatus = "dead"
)

type EnqueueRequest struct {
	SourceID       source.ID
	Trigger        Trigger
	Payload        json.RawMessage
	IdempotencyKey string
	Priority       int
}

type Acquisition struct {
	ID             AcquisitionID     `json:"id"`
	SourceID       source.ID         `json:"source_id"`
	Trigger        Trigger           `json:"trigger"`
	Payload        json.RawMessage   `json:"-"`
	IdempotencyKey string            `json:"idempotency_key"`
	Status         AcquisitionStatus `json:"status"`
	Priority       int               `json:"priority"`
	Attempts       int               `json:"attempts"`
	AvailableAt    time.Time         `json:"available_at"`
	RequestedAt    time.Time         `json:"requested_at"`
	LeaseOwner     string            `json:"-"`
	LeaseUntil     *time.Time        `json:"lease_until,omitempty"`
	LastError      string            `json:"last_error,omitempty"`
}
