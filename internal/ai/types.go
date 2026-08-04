package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type JobKind string

const (
	JobKindStorySummary JobKind = "story_summary"
	JobKindDigest       JobKind = "digest"
)

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobRetry     JobStatus = "retry"
	JobCompleted JobStatus = "completed"
	JobPartial   JobStatus = "partial"
	JobFailed    JobStatus = "failed"
	JobDead      JobStatus = "dead"
)

type ArtifactStatus string

const (
	StatusNotRequested ArtifactStatus = "not_requested"
	StatusQueued       ArtifactStatus = "queued"
	StatusRunning      ArtifactStatus = "running"
	StatusCompleted    ArtifactStatus = "completed"
	StatusPartial      ArtifactStatus = "partial"
	StatusFailed       ArtifactStatus = "failed"
	StatusStale        ArtifactStatus = "stale"
	StatusUnavailable  ArtifactStatus = "unavailable"
)

const (
	StorySummaryPromptVersion = "story-summary-v1"
	DigestPromptVersion       = "catch-up-digest-title-only-v1"
)

var (
	ErrNoJob       = errors.New("no AI job available")
	ErrNotFound    = errors.New("AI artifact not found")
	ErrUnavailable = errors.New("AI Provider is unavailable")
	ErrNoStories   = errors.New("no unread Stories match the Digest scope")
	ErrQueueFull   = errors.New("AI job queue is full")
)

type ScopeValidationError struct {
	Field   string
	Message string
}

func (err *ScopeValidationError) Error() string {
	return err.Message
}

type QueueLimitError struct {
	Limit int
}

func (err *QueueLimitError) Error() string {
	return fmt.Sprintf("AI job queue is full (limit %d active jobs)", err.Limit)
}

func (err *QueueLimitError) Unwrap() error {
	return ErrQueueFull
}

type ScopeLimitError struct {
	Count int
	Limit int
}

func (err *ScopeLimitError) Error() string {
	return fmt.Sprintf("Digest scope contains more than the configured limit of %d Stories", err.Limit)
}

type ProviderMetadata struct {
	Name  string `json:"provider"`
	Model string `json:"model"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GenerateRequest struct {
	Messages    []Message
	MaxTokens   int
	Temperature *float32
	JSONMode    bool
}

type GenerateResponse struct {
	Content          string
	Model            string
	PromptTokens     int
	CompletionTokens int
}

type Provider interface {
	Metadata() ProviderMetadata
	Generate(context.Context, GenerateRequest) (GenerateResponse, error)
}

type StoryEntrySnapshot struct {
	Label       string     `json:"label"`
	EntryID     string     `json:"entry_id"`
	SourceTitle string     `json:"source_title"`
	Title       string     `json:"title"`
	Author      string     `json:"author,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	Content     string     `json:"content,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

type StorySnapshot struct {
	StoryID               string               `json:"story_id"`
	Title                 string               `json:"title"`
	Entries               []StoryEntrySnapshot `json:"entries"`
	MembershipFingerprint string               `json:"membership_fingerprint,omitempty"`
	InputFingerprint      string               `json:"input_fingerprint"`
}

type DigestStorySnapshot struct {
	Label            string     `json:"label"`
	StoryID          string     `json:"story_id"`
	Title            string     `json:"title"`
	SourceTitle      string     `json:"source_title,omitempty"`
	EntryCount       int        `json:"entry_count"`
	SourceCount      int        `json:"source_count"`
	SortTime         *time.Time `json:"sort_time,omitempty"`
	InputFingerprint string     `json:"input_fingerprint,omitempty"`
}

type DigestScope struct {
	StartAt    *time.Time `json:"start_at,omitempty"`
	EndAt      *time.Time `json:"end_at,omitempty"`
	MaxStories int        `json:"max_stories,omitempty"`
}

type DigestPreview struct {
	Scope                    DigestScope `json:"scope"`
	MatchingStories          int         `json:"matching_stories"`
	MatchingStoriesTruncated bool        `json:"matching_stories_truncated"`
	SelectedStories          int         `json:"selected_stories"`
	SafetyLimit              int         `json:"safety_limit"`
	CanQueue                 bool        `json:"can_queue"`
}

type Job struct {
	ID          string          `json:"id"`
	Kind        JobKind         `json:"kind"`
	TargetID    string          `json:"target_id"`
	Payload     json.RawMessage `json:"-"`
	Status      JobStatus       `json:"status"`
	Attempts    int             `json:"attempts"`
	AvailableAt time.Time       `json:"available_at"`
	RequestedAt time.Time       `json:"requested_at"`
	LeaseOwner  string          `json:"-"`
	LeaseUntil  *time.Time      `json:"-"`
	LastError   string          `json:"last_error,omitempty"`
}

type JobReceipt struct {
	ID       string    `json:"id"`
	Kind     JobKind   `json:"kind"`
	TargetID string    `json:"target_id"`
	Status   JobStatus `json:"status"`
}

type SummarySource struct {
	Label       string `json:"label"`
	EntryID     string `json:"entry_id"`
	Title       string `json:"title"`
	SourceTitle string `json:"source_title,omitempty"`
	Note        string `json:"note"`
}

type StorySummary struct {
	StoryID          string          `json:"story_id"`
	Status           ArtifactStatus  `json:"status"`
	Overview         string          `json:"overview,omitempty"`
	KeyPoints        []string        `json:"key_points,omitempty"`
	Sources          []SummarySource `json:"sources,omitempty"`
	Provider         string          `json:"provider,omitempty"`
	Model            string          `json:"model,omitempty"`
	PromptVersion    string          `json:"prompt_version,omitempty"`
	InputFingerprint string          `json:"input_fingerprint,omitempty"`
	Error            string          `json:"error,omitempty"`
	CreatedAt        time.Time       `json:"created_at,omitempty"`
	UpdatedAt        time.Time       `json:"updated_at,omitempty"`
}

type DigestTheme struct {
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	StoryIDs []string `json:"story_ids"`
}

type DigestPriority struct {
	Rank     int      `json:"rank"`
	Title    string   `json:"title"`
	Reason   string   `json:"reason"`
	StoryIDs []string `json:"story_ids"`
}

type DigestStory struct {
	Label       string `json:"label"`
	StoryID     string `json:"story_id"`
	Title       string `json:"title"`
	EntryCount  int    `json:"entry_count"`
	SourceCount int    `json:"source_count"`
	Available   bool   `json:"available"`
}

type DigestOmission struct {
	Label   string `json:"label"`
	StoryID string `json:"story_id,omitempty"`
	Title   string `json:"title"`
	Reason  string `json:"reason"`
}

type Digest struct {
	ID               string           `json:"id"`
	Status           ArtifactStatus   `json:"status"`
	Mode             string           `json:"mode"`
	StoryCount       int              `json:"story_count"`
	StartAt          *time.Time       `json:"start_at,omitempty"`
	EndAt            *time.Time       `json:"end_at,omitempty"`
	Overview         string           `json:"overview,omitempty"`
	Themes           []DigestTheme    `json:"themes,omitempty"`
	Priorities       []DigestPriority `json:"priorities,omitempty"`
	Stories          []DigestStory    `json:"stories,omitempty"`
	Omissions        []DigestOmission `json:"omissions,omitempty"`
	Provider         string           `json:"provider,omitempty"`
	Model            string           `json:"model,omitempty"`
	PromptVersion    string           `json:"prompt_version,omitempty"`
	InputFingerprint string           `json:"input_fingerprint,omitempty"`
	Error            string           `json:"error,omitempty"`
	CreatedAt        time.Time        `json:"created_at,omitempty"`
	UpdatedAt        time.Time        `json:"updated_at,omitempty"`
}

type GeneratedStorySummary struct {
	Overview  string
	KeyPoints []string
	Sources   []SummarySource
}

type GeneratedDigestTheme struct {
	Title       string
	Summary     string
	StoryLabels []string
}

type GeneratedDigestPriority struct {
	Rank        int
	Title       string
	Reason      string
	StoryLabels []string
}

type GeneratedDigest struct {
	Overview      string
	Themes        []GeneratedDigestTheme
	Priorities    []GeneratedDigestPriority
	OmittedLabels []string
}

type Store interface {
	SnapshotStory(context.Context, string) (StorySnapshot, error)
	SnapshotUnreadStories(context.Context, DigestScope) ([]DigestStorySnapshot, error)
	GetStorySummary(context.Context, string) (StorySummary, error)
	EnqueueStorySummary(context.Context, StorySnapshot, ProviderMetadata) (StorySummary, JobReceipt, error)
	GetDigest(context.Context, string) (Digest, error)
	ListDigests(context.Context, int) ([]Digest, error)
	EnqueueDigest(context.Context, DigestScope, []DigestStorySnapshot, string, ProviderMetadata) (Digest, JobReceipt, error)
	Claim(context.Context, string, time.Duration) (Job, error)
	CompleteStorySummary(context.Context, Job, string, GeneratedStorySummary, ProviderMetadata) error
	CompleteDigest(context.Context, Job, string, GeneratedDigest, ProviderMetadata) error
	Retry(context.Context, Job, string, time.Time, error) error
	Fail(context.Context, Job, string, error) error
}

type Summarization interface {
	RequestStorySummary(context.Context, string) (JobReceipt, error)
	GetStorySummary(context.Context, string) (StorySummary, error)
	PreviewDigest(context.Context, DigestScope) (DigestPreview, error)
	RequestDigest(context.Context, DigestScope) (JobReceipt, error)
	ListDigests(context.Context, int) ([]Digest, error)
	GetDigest(context.Context, string) (Digest, error)
}
