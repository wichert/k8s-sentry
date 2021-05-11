/*
Copyright 2019 Wichert Akkerman

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

// DefaultEventHandler is the default handler for events.
type DefaultEventHandler struct {
	Event *v1.Event
}

// Fingerprint returns the fingerprint entries that are specific for an event typeX
func (h DefaultEventHandler) Fingerprint() []string {
	return []string{
		h.Event.InvolvedObject.APIVersion,
		h.Event.InvolvedObject.Kind,
		h.Event.InvolvedObject.Namespace,
		mangleName(h.Event.InvolvedObject.Name),
		h.Event.InvolvedObject.FieldPath,
	}
}

// Tags returns a set of tags that should be added to the event
func (h DefaultEventHandler) Tags() map[string]string {
	return h.Event.GetLabels()
}

// NewDefaultEventHandler creates a new DefaultEventHandler instance
func NewDefaultEventHandler(app *application, evt *v1.Event) EventHandler {
	return &DefaultEventHandler{Event: evt}
}

func fingerprintFromMeta(resource *metav1.ObjectMeta) []string {
	// If the object has a controller owner, use that for grouping purposes.
	for _, owner := range resource.OwnerReferences {
		if owner.Controller != nil && *owner.Controller {
			return []string{
				owner.APIVersion,
				owner.Kind,
				owner.Name,
			}
		}
	}

	name := resource.GenerateName
	if name == "" {
		name = mangleName(resource.Name)
	}

	// Otherwise we group based onthe object itself
	return []string{
		resource.Namespace,
		name,
	}
}

func mangleName(original string) string {
	splits := strings.Split(original, "-")

	if len(splits) < 3 {
		return splits[0]
	}

	return strings.Join(splits[0:len(splits)-2], "-")
}
