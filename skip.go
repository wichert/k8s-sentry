package main

import (
	v1 "k8s.io/api/core/v1"
	"strings"
)

const SkipLevelKey string = "secunet.sentry/skip-event-levels"

func trim(mess []string) []string {

	var result []string

	for _, val := range mess {
		result = append(result, strings.TrimSpace(val))
	}

	return result
}

func parseSkipLevels(raw *string, fallback ...string) []string {

	if raw == nil || len(strings.TrimSpace(*raw)) == 0 {

		var dflt []string

		for _, val := range fallback {
			dflt = append(dflt, strings.ToLower(val))
		}
		return trim(dflt)

	} else {
		return trim(strings.Split(strings.ToLower(*raw), ","))
	}

}

func skipEvent(evt *v1.Event, nsSkipLevels map[string][]string, defaultSkipLevels []string) bool {

	evtType := strings.ToLower(evt.Type)
	evtNs := evt.Namespace

	appliedSkipLevels, hasNsMapping := nsSkipLevels[evt.Namespace]

	if len(evtNs) == 0 || !hasNsMapping {
		appliedSkipLevels = defaultSkipLevels
	}

	for _, level := range appliedSkipLevels {
		if level == evtType {
			return true
		}
	}

	return false
}
