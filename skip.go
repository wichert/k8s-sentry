package main

import (
	v1 "k8s.io/api/core/v1"
	"log"
	"strconv"
	"strings"
)

const SkipLevelKey string = "sentry/skip-event-levels"
const SkipReasonKey string = "sentry/skip-event-reasons"
const SkipPodModificationEvent string = "sentry/ignore-pod-updates"

const SkipEventReasonsEnv = "SKIP_EVENT_REASONS"
const SkipEventLevelsEnv = "SKIP_EVENT_LEVELS"

type SkipCriteria int

const (
	SKIP_BY_REASON SkipCriteria = iota
	SKIP_BY_LEVEL  SkipCriteria = iota
)

func trim(mess []string) []string {

	var result []string

	for _, val := range mess {
		result = append(result, strings.TrimSpace(val))
	}

	return result
}

func skipConfigLookupKey(criteriaType SkipCriteria, resourceType string, criteriaValue string) string {

	var result strings.Builder

	result.WriteString(strconv.Itoa(int(criteriaType)))
	if len(resourceType) > 0 {
		result.WriteString("-")
		result.WriteString(resourceType)
	}

	result.WriteString("-")
	result.WriteString(criteriaValue)

	return strings.ToLower(result.String())
}

// Parses SkipConfig declarations from either environment variables or namespace annotation values. The supported
// declaration format is: [involvedObjectType:]skipCriteria[,...].
//
// Some examples:
//
// Filter Pod, Service and PV events by their reason field:
// Set env SkipEventReasonsEnv or the namespace annotation SkipReasonKey to e.g.:
//   Pod:created,Service:AllocationFailed,PersistentVolume:PersistentVolumeDeleted
//
// Filter Pod, Service and PV events by their level:
// Set env SkipEventLevelsEnv or the namespace annotation SkipLevelKey to e.g.:
//   Pod:normal,Service:warning,PersistentVolume:normal
//
// Generally filter events with level normal, while selectively also filter warnings related to Pods or Services:
// Set env SkipEventLevelsEnv or the namespace annotation SkipLevelKey to:
//   normal,Pod:warning,Service:warning
//
// To find out which combinations of event level, object and reason currently exist, run:
//   kubectl get events --all-namespaces -o custom-columns=OTYPE:.involvedObject.kind,LEVEL:.type,REASON:.reason,NAMESPACE:.metadata.namespace | sort | uniq
//
func parseSkipConfig(criteriaType SkipCriteria, raw *string, fallback ...string) []string {

	var criteriaList []string

	if raw == nil || len(strings.TrimSpace(*raw)) == 0 {

		return fallback

	} else {
		criteriaList = trim(strings.Split(strings.ToLower(*raw), ","))
	}

	var result []string

	for _, criteria := range criteriaList {

		// expecting format of either "resourceType:criteria" or just "criteria"
		// E.g.
		// "Pod:created" means, if criteriaType is set to SKIP_BY_REASON:
		// filter events whose involved object is a pod and the reason is "created"
		// ":created" or "created" means, if criteriaType is set to SKIP_BY_REASON:
		// filter ALL events of reason "created"
		// ... the same applies for filtering by level, so:
		// "ConfigMap:normal" means, if criteriaType is set to SKIP_BY_LEVEL:
		// filter events whose involved object is a configmap and the event level is "normal"
		typeAndCriteria := trim(strings.Split(strings.ToLower(criteria), ":"))

		if len(typeAndCriteria) > 2 || len(typeAndCriteria) == 0 {
			// declaration error, more than one ":" delimiter not supported
			log.Printf("Illegal skip event config declaration, ignoring: %s\n", criteria)
			continue
		}

		var resourceType string
		var criteriaValue string

		if len(typeAndCriteria) == 2 {
			resourceType = strings.TrimSpace(strings.ToLower(typeAndCriteria[0]))
			criteriaValue = strings.TrimSpace(strings.ToLower(typeAndCriteria[1]))
		} else {
			criteriaValue = strings.TrimSpace(strings.ToLower(strings.TrimSpace(typeAndCriteria[0])))
		}

		result = append(result, skipConfigLookupKey(criteriaType, resourceType, criteriaValue))
	}

	return result
}

func skipEvent(evt *v1.Event, nsSkipLevels map[string]map[string]struct{}) bool {

	ns := evt.Namespace
	reason := strings.ToLower(evt.Reason)
	level := strings.ToLower(evt.Type)
	oType := strings.ToLower(evt.InvolvedObject.Kind)

	appliedSkipLevels, hasNsMapping := nsSkipLevels[ns]

	if len(ns) == 0 || !hasNsMapping {
		appliedSkipLevels = nsSkipLevels[AnyNS]
	}

	_, hasOtypeSpecificReasonFilter := appliedSkipLevels[skipConfigLookupKey(SKIP_BY_REASON, oType, reason)]
	if hasOtypeSpecificReasonFilter {
		return true
	}

	_, hasOtypeAgnosticReasonFilter := appliedSkipLevels[skipConfigLookupKey(SKIP_BY_REASON, "", reason)]
	if hasOtypeAgnosticReasonFilter {
		return true
	}

	_, hasOtypeSpecificLevelFilter := appliedSkipLevels[skipConfigLookupKey(SKIP_BY_LEVEL, oType, level)]
	if hasOtypeSpecificLevelFilter {
		return true
	}

	_, hasOtypeAgnosticLevelFilter := appliedSkipLevels[skipConfigLookupKey(SKIP_BY_LEVEL, "", level)]
	if hasOtypeAgnosticLevelFilter {
		return true
	}

	return false
}
