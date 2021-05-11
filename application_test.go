package main

import (
	"os"
	"testing"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/core/v1"
)

func TestSkipEvent(t *testing.T) {
	t.Parallel()

	evt := &v1.Event{Type: v1.EventTypeNormal}

	skipLevels := map[string]map[string]struct{}{
		"default": {
			skipConfigLookupKey(SKIP_BY_REASON, "Pod", "created"):    {},
			skipConfigLookupKey(SKIP_BY_REASON, "", "puLLeD"):        {},
			skipConfigLookupKey(SKIP_BY_LEVEL, "Service", "warning"): {},
			skipConfigLookupKey(SKIP_BY_LEVEL, "", "info"):           {},
		},
	}

	evt.Type = v1.EventTypeNormal
	evt.ObjectMeta.Namespace = "default"
	evt.InvolvedObject.Kind = "Pod"
	evt.Reason = "created"
	if !skipEvent(evt, skipLevels) {
		t.Error("pod:created should be skipped by reason")
	}

	evt.Reason = "pulled"
	if !skipEvent(evt, skipLevels) {
		t.Error("[any kind]:pulled should be skipped by reason")
	}

	evt.InvolvedObject.Kind = "serVICE"
	evt.Type = v1.EventTypeWarning
	if !skipEvent(evt, skipLevels) {
		t.Error("service:warning should be skipped by level")
	}

	evt.InvolvedObject.Kind = "ConfigMap"
	evt.Type = v1.EventTypeNormal
	if !skipEvent(evt, skipLevels) {
		t.Error("[any kind]:info should be skipped by level")
	}

	evt.Type = "Unknown"
	evt.Reason = "killed"
	if skipEvent(evt, skipLevels) {
		t.Error("Unknown event types must not be skipped")
	}
}

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

func TestInCluster(t *testing.T) {
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("KUBERNETES_SERVICE_PORT")

	if inCluster() {
		t.Error("inCluster returns true if Kubernetes service env is missing")
	}

	os.Setenv("KUBERNETES_SERVICE_HOST", "api")
	if inCluster() {
		t.Error("inCluster returns true if KUBERNETES_SERVICE_PORT is missing")
	}

	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Setenv("KUBERNETES_SERVICE_PORT", "4138")
	if inCluster() {
		t.Error("inCluster returns true if KUBERNETES_SERVICE_HOST is missing")
	}

	os.Setenv("KUBERNETES_SERVICE_HOST", "api")
	os.Setenv("KUBERNETES_SERVICE_PORT", "4138")
	if !inCluster() {
		t.Error("inCluster returns false with Kubernetes service env present")
	}

	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("KUBERNETES_SERVICE_PORT")
}
