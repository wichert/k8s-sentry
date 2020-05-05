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
)

// EventHandler defines handlers for specific event types
type EventHandler interface {
	// Fingerprint returns the fingerprint entries that are specific for an event type
	Fingerprint() []string

	// Tags returns a set of tags that should be added to the event
	Tags() map[string]string
}

type registryKey struct {
	APIVersion string
	Kind       string
}

var handlerRegistry = map[registryKey]func(*application, *v1.Event) EventHandler{
	{APIVersion: "v1", Kind: "Pod"}: NewPodEventHandler,
}

// NewEventHandler is a factory function to create the right EventHandler for an event
func NewEventHandler(app *application, evt *v1.Event) EventHandler {
	key := registryKey{APIVersion: evt.InvolvedObject.APIVersion, Kind: evt.InvolvedObject.Kind}
	factory := handlerRegistry[key]
	if factory != nil {
		if handler := factory(app, evt); handler != nil {
			return handler
		}
	}
	return NewDefaultEventHandler(app, evt)
}
