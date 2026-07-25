package main

import (
	"context"
	"os"
	"testing"

	"github.com/wenpengfei/pulse/internal/config"
)

func TestRunContextStartsMigratesAndStops(t *testing.T) {
	databaseURL := os.Getenv("PULSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PULSE_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- runContext(ctx, config.Config{
			HTTPAddr:    "127.0.0.1:0",
			DatabaseURL: databaseURL,
			Roles:       []config.Role{config.RoleWeb},
		}, ready)
	}()
	<-ready
	cancel()
	if err := <-result; err != nil {
		t.Fatalf("runContext() error = %v", err)
	}
}

func TestRunContextWithoutWebStopsOnCancellation(t *testing.T) {
	databaseURL := os.Getenv("PULSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PULSE_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- runContext(ctx, config.Config{
			DatabaseURL: databaseURL,
			Roles:       []config.Role{config.RoleScheduler},
		}, ready)
	}()
	<-ready
	cancel()
	if err := <-result; err != nil {
		t.Fatalf("runContext() error = %v", err)
	}
}
