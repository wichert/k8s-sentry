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
	"fmt"
	"log"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type application struct {
	clientset          *kubernetes.Clientset
	defaultEnvironment string
	namespace          string
}

func (app application) Run() chan struct{} {
	if app.namespace == "" {
		app.namespace = v1.NamespaceAll
	}

	watchList := cache.NewListWatchFromClient(
		app.clientset.CoreV1().RESTClient(),
		"events",
		app.namespace,
		fields.Everything(),
	)
	_, controller := cache.NewInformer(
		watchList,
		&v1.Event{},
		time.Second*30,
		cache.ResourceEventHandlerFuncs{
			AddFunc: app.handleEventAdd,
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)
	return stop
}

func (app application) handleEventAdd(obj interface{}) {
	evt, ok := obj.(*v1.Event)
	if !ok {
		sentry.CaptureMessage("Unexpected event type")
		return
	}

	if skipEvent(evt) {
		return
	}

	sentryEvent := sentry.NewEvent()
	if app.defaultEnvironment != "" {
		sentryEvent.Environment = app.defaultEnvironment
	} else {
		sentryEvent.Environment = evt.InvolvedObject.Namespace
	}

	sentryEvent.Message = fmt.Sprintf("%s/%s: %s", evt.InvolvedObject.Kind, evt.InvolvedObject.Name, evt.Message)
	sentryEvent.Level = getSentryLevel(evt)
	sentryEvent.Timestamp = evt.ObjectMeta.CreationTimestamp.Unix()
	sentryEvent.Fingerprint = []string{
		evt.Source.Component,
		evt.Type,
		evt.Reason,
		evt.Message,
	}

	sentryEvent.Tags["namespace"] = evt.InvolvedObject.Namespace
	sentryEvent.Tags["component"] = evt.Source.Component
	if evt.ClusterName != "" {
		sentryEvent.Tags["cluster"] = evt.ClusterName
	}
	sentryEvent.Tags["reason"] = evt.Reason
	sentryEvent.Tags["kind"] = evt.InvolvedObject.Kind
	sentryEvent.Tags["type"] = evt.Type

	if evt.Action != "" {
		sentryEvent.Extra["action"] = evt.Action
	}
	sentryEvent.Extra["count"] = evt.Count

	handler := NewEventHandler(&app, evt)
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, handler.Fingerprint()...)

	log.Printf("%s %s\n", evt.Type, sentryEvent.Message)
	sentry.CaptureEvent(sentryEvent)
}

func skipEvent(evt *v1.Event) bool {
	return evt.Type == v1.EventTypeNormal
}

func getSentryLevel(evt *v1.Event) sentry.Level {
	switch evt.Type {
	case v1.EventTypeWarning:
		return sentry.LevelWarning
	case "Error":
		return sentry.LevelError
	default:
		fmt.Printf("Unexpected event type: %v\n", evt.Type)
		return sentry.LevelInfo
	}
}

func getEventFingerprint(evt *v1.Event) []string {
	return []string{
		evt.Source.Component,
		evt.InvolvedObject.APIVersion,
		evt.InvolvedObject.Kind,
		evt.InvolvedObject.Namespace,
		evt.InvolvedObject.Name,
		evt.InvolvedObject.FieldPath,
		evt.Type,
		evt.Reason,
		evt.Message,
	}
}

func inCluster() bool {
	return os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != ""
}
