package effect

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/rule"
)

type fakeStore struct {
	effect    rule.Effect
	claimErr  error
	succeeded bool
	retried   bool
}

func (store *fakeStore) Claim(context.Context, string, time.Duration) (rule.Effect, error) {
	return store.effect, store.claimErr
}
func (store *fakeStore) Succeed(context.Context, rule.Effect, string) error {
	store.succeeded = true
	return nil
}
func (store *fakeStore) Retry(context.Context, string, string, time.Time, error) error {
	store.retried = true
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestProcessorDeliversWebhookAndCompletesEffect(t *testing.T) {
	store := &fakeStore{effect: rule.Effect{
		ID: "effect", RuleID: "rule", RuleVersion: 2, EntryID: entry.ID("entry"),
		Kind: rule.ActionWebhook, Value: "https://hooks.example/pulse",
	}}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != store.effect.Value {
			t.Errorf("request = %s %s", request.Method, request.URL)
		}
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), `"entry_id":"entry"`) {
			t.Errorf("body = %s", body)
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}

	if err := NewProcessor(store, client).ProcessNext(context.Background(), "worker"); err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if !store.succeeded || store.retried {
		t.Errorf("succeeded=%v retried=%v", store.succeeded, store.retried)
	}
}

func TestProcessorRetriesFailedWebhook(t *testing.T) {
	store := &fakeStore{effect: rule.Effect{
		ID: "effect", Kind: rule.ActionWebhook, Value: "https://hooks.example/pulse",
	}}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}
	if err := NewProcessor(store, client).ProcessNext(context.Background(), "worker"); err == nil {
		t.Fatal("ProcessNext() error = nil")
	}
	if !store.retried || store.succeeded {
		t.Errorf("succeeded=%v retried=%v", store.succeeded, store.retried)
	}
}

func TestProcessorCompletesInAppNotificationWithoutHTTP(t *testing.T) {
	store := &fakeStore{effect: rule.Effect{
		ID: "effect", Kind: rule.ActionNotification, Value: "New item",
	}}
	if err := NewProcessor(store, nil).ProcessNext(context.Background(), "worker"); err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if !store.succeeded {
		t.Fatal("notification was not completed")
	}
}
