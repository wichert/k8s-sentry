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
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	lru "github.com/hashicorp/golang-lru"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type terminationKey struct {
	podUID        types.UID
	containerName string
	restartCount  int32
}

type application struct {
	clientset             *kubernetes.Clientset
	defaultEnvironment    string
	globalSkipEventLevels []string
	nsSkipEventLevels     map[string][]string
	terminationsSeen      *lru.Cache
	namespaces            []string
}

func (app *application) Run() (chan struct{}, error) {
	terminationsSeen, err := lru.New(500)
	if err != nil {
		return nil, err
	}
	app.terminationsSeen = terminationsSeen

	stop := make(chan struct{})

	errGroup, ctx := errgroup.WithContext(context.Background())

	app.nsSkipEventLevels = make(map[string][]string)
	errGroup.Go(func() error {
		return app.monitorNamespaces(stop, ctx, func() {
			for _, namespace := range app.namespaces {
				go app.monitorEvents(namespace, stop)
				go app.monitorPods(namespace, stop)
			}
		})
	})

	return stop, errGroup.Wait()
}

func (app application) monitorNamespaces(stop chan struct{}, ctx context.Context, readyFn func()) error {

	client := app.clientset.CoreV1().RESTClient()

	optionsModifier := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.Everything().String()
	}

	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		optionsModifier(&options)
		return client.Get().
			Resource("namespaces").
			VersionedParams(&options, metav1.ParameterCodec).
			Do().
			Get()
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true
		optionsModifier(&options)
		return client.Get().
			Resource("namespaces").
			VersionedParams(&options, metav1.ParameterCodec).
			Watch()
	}
	watchList := &cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc}

	_, controller := cache.NewInformer(
		watchList,
		&v1.Namespace{},
		time.Second*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    app.handleNsAdd,
			UpdateFunc: app.handleNsUpdate,
			DeleteFunc: app.handleNsDelete,
		},
	)

	go controller.Run(stop)

	err := wait.PollImmediate(250*time.Millisecond, time.Duration(20)*time.Second, func() (bool, error) {
		return controller.HasSynced(), nil
	})

	if err != nil {
		log.Printf("Timeout while initializing namespaces, unable to proceed.")
		return ctx.Err()

	} else {
		readyFn()
		return nil
	}
}

func (app application) monitorPods(namespace string, stop chan struct{}) {
	watchList := cache.NewListWatchFromClient(
		app.clientset.CoreV1().RESTClient(),
		"pods",
		namespace,
		fields.Everything(),
	)

	_, controller := cache.NewInformer(
		watchList,
		&v1.Pod{},
		time.Second*30,
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: app.handlePodUpdate,
		},
	)

	controller.Run(stop)
}

func (app application) monitorEvents(namespace string, stop chan struct{}) {

	watchList := cache.NewListWatchFromClient(
		app.clientset.CoreV1().RESTClient(),
		"events",
		namespace,
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

	controller.Run(stop)
}

func (app *application) handleNsAdd(newNS interface{}) {
	app.handleNsUpdate(nil, newNS)
}

func (app *application) handleNsUpdate(_, newNS interface{}) {

	ns, _ := newNS.(*v1.Namespace)

	value, _ := ns.ObjectMeta.Annotations[SkipLevelKey]

	skipLevels := parseSkipLevels(&value, app.globalSkipEventLevels...)
	app.nsSkipEventLevels[ns.Name] = skipLevels

}

func (app *application) handleNsDelete(goneNS interface{}) {

	ns, _ := goneNS.(*v1.Namespace)

	delete(app.nsSkipEventLevels, ns.Name)
}

func (app *application) handlePodUpdate(oldObj, newObj interface{}) {
	pod, ok := newObj.(*v1.Pod)
	if !ok {
		sentry.CaptureMessage("Unexpected pod type")
		return
	}

	var sentryEvent *sentry.Event

	// The Pod is still running. Check if one of its containers terminated with a non-zero exit code.
	// If so report that as an error.
	// Note that this will fail if multiple containers in the pod are terminating at the same time.
	// Since that should be rare, and hopefully someone will investigate on any error anyway we
	// ignore that situation (for now).
	allContainers := append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...)
	for _, status := range allContainers {
		if app.isNewTermination(pod, &status) && status.State.Terminated.ExitCode != 0 {
			// Note that we only care about terminations in the last half seconds. This prevents
			// us from treating updates for other reasons after a container terminated as another
			// occurance of the termination.
			sentryEvent = sentry.NewEvent()
			sentryEvent.Message = status.State.Terminated.Message
			sentryEvent.ServerName = pod.Spec.NodeName
			if sentryEvent.Message == "" {
				if status.State.Terminated.Reason == "Error" {
					sentryEvent.Message = fmt.Sprintf("Error %s exited with code %d", status.Name, status.State.Terminated.ExitCode)
				} else {
					// OOMKilled does not leave a message :(
					sentryEvent.Message = status.State.Terminated.Reason
				}
			}

			sentryEvent.Release = status.Image
			sentryEvent.Tags["reason"] = status.State.Terminated.Reason
			sentryEvent.Extra["exit-code"] = strconv.FormatInt(int64(status.State.Terminated.ExitCode), 10)
			sentryEvent.Extra["restartCount"] = status.RestartCount
			sentryEvent.Extra["restartPolicy"] = pod.Spec.RestartPolicy
			sentryEvent.Extra["container"] = status.Name
			sentryEvent.Extra["pod"] = pod.Name

			sentryEvent.Extra["pod-phase"] = pod.Status.Phase
			if pod.Status.Message != "" {
				sentryEvent.Extra["pod-status-message"] = pod.Status.Message
			}
			if pod.Status.Reason != "" {
				sentryEvent.Extra["pod-status-reason"] = pod.Status.Reason
			}

			sentryEvent.Logger = "kubernetes"
			sentryEvent.Level = sentry.LevelError
			if app.defaultEnvironment != "" {
				sentryEvent.Environment = app.defaultEnvironment
			} else {
				sentryEvent.Environment = pod.Namespace
			}

			sentryEvent.Fingerprint = append(
				[]string{
					sentryEvent.Tags["reason"],
				},
				fingerprintFromMeta(&pod.ObjectMeta)...)

			sentryEvent.Tags["namespace"] = pod.Namespace
			if pod.ClusterName != "" {
				sentryEvent.Tags["cluster"] = pod.ClusterName
			}
			sentryEvent.Tags["kind"] = pod.Kind
			for k, v := range pod.ObjectMeta.Labels {
				sentryEvent.Tags[k] = v
			}
			sentryEvent.Message = fmt.Sprintf("Pod/%s: %s", pod.Name, sentryEvent.Message)

			sentry.CaptureEvent(sentryEvent)
		}
	}
}

func (app *application) isNewTermination(pod *v1.Pod, status *v1.ContainerStatus) bool {

	if status.State.Terminated == nil {
		return false
	}

	finishedAt := status.State.Terminated.FinishedAt

	key := terminationKey{
		podUID:        pod.UID,
		containerName: status.Name,
		restartCount:  status.RestartCount,
	}
	_, seen := app.terminationsSeen.Get(key)
	app.terminationsSeen.Add(key, finishedAt)

	return !seen
}

func (app application) handleEventAdd(obj interface{}) {
	evt, ok := obj.(*v1.Event)
	if !ok {
		sentry.CaptureMessage("Unexpected event type")
		return
	}

	if skipEvent(evt, app.nsSkipEventLevels, app.globalSkipEventLevels) {
		return
	}

	sentryEvent := sentry.NewEvent()
	if app.defaultEnvironment != "" {
		sentryEvent.Environment = app.defaultEnvironment
	} else {
		sentryEvent.Environment = evt.InvolvedObject.Namespace
	}

	msg := evt.Message
	if msg == "" {
		msg = evt.Reason
	}

	sentryEvent.Logger = "kubernetes"
	sentryEvent.Message = fmt.Sprintf("%s/%s: %s", evt.InvolvedObject.Kind, evt.InvolvedObject.Name, msg)
	sentryEvent.Level = getSentryLevel(evt)
	sentryEvent.Timestamp = evt.ObjectMeta.CreationTimestamp.Time
	sentryEvent.Fingerprint = []string{
		evt.Source.Component,
		evt.Type,
		evt.Reason,
	}

	sentryEvent.Tags["namespace"] = evt.InvolvedObject.Namespace
	sentryEvent.Tags["component"] = evt.Source.Component
	if evt.ClusterName != "" {
		sentryEvent.Tags["cluster"] = evt.ClusterName
	}
	sentryEvent.Tags["reason"] = evt.Reason
	sentryEvent.Tags["kind"] = evt.InvolvedObject.Kind
	sentryEvent.Tags["type"] = evt.Type
	if evt.ReportingController != "" {
		sentryEvent.Tags["controller"] = evt.ReportingController
	}
	if evt.Action != "" {
		sentryEvent.Extra["action"] = evt.Action
	}
	sentryEvent.Extra["count"] = evt.Count

	handler := NewEventHandler(&app, evt)
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, handler.Fingerprint()...)
	for k, v := range handler.Tags() {
		sentryEvent.Tags[k] = v
	}

	log.Printf("%s %s\n", evt.Type, sentryEvent.Message)
	sentry.CaptureEvent(sentryEvent)
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

func inCluster() bool {
	return os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != ""
}
