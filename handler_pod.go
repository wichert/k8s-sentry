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
	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodEventHandler handles events involved Pods.
type PodEventHandler struct {
	Pod   *v1.Pod
	Event *v1.Event
}

// Fingerprint returns the fingerprint entries that are specific for an event typeX
func (h PodEventHandler) Fingerprint() []string {
	return fingerprintFromMeta(&h.Pod.ObjectMeta)
}

// Tags returns a set of tags that should be added to the event
func (h PodEventHandler) Tags() map[string]string {
	tags := make(map[string]string)
	for k, v := range h.Pod.Labels {
		tags[k] = v
	}
	tags["nodeName"] = h.Pod.Spec.NodeName
	return tags
}

// NewPodEventHandler creates a new PodEventHandler instance
func NewPodEventHandler(app *application, evt *v1.Event) EventHandler {
	pod, err := app.clientset.CoreV1().Pods(evt.Namespace).Get(
		evt.InvolvedObject.Name,
		metav1.GetOptions{
			ResourceVersion: evt.InvolvedObject.ResourceVersion,
		},
	)
	if err != nil {
		sentry.CaptureException(err)
		return nil
	}
	return &PodEventHandler{Pod: pod, Event: evt}
}
