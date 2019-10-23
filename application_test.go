package main

import (
	"testing"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/core/v1"
)

func TestGetSentryLevel(t *testing.T) {
	t.Parallel()

	evt := &v1.Event{Type: "Warning"}
	if getSentryLevel(evt) != sentry.LevelWarning {
		t.Error("Type Warning not reported with warning level")
	}

	evt.Type = "Error"
	if getSentryLevel(evt) != sentry.LevelError {
		t.Error("Type Error not reported with error level")
	}

	evt.Type = "Other"
	if getSentryLevel(evt) != sentry.LevelInfo {
		t.Error("Unknown event types not reported with info level")
	}

}
