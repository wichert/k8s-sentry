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
	"flag"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var configFlag = flag.String("kubeconfig", "", "Configuration file")
var defaultEnvironment = os.Getenv("SENTRY_ENVIRONMENT")

func main() {
	flag.Parse()

	if defaultEnvironment == "" {
		defaultEnvironment = os.Getenv("ENVIRONMENT")
		if defaultEnvironment != "" {
			log.Println("Warning: ENVIRONMENT environment variable has been deprecated. Please use SENTRY_ENVIRONMENT instead.")
		}
	}
	if os.Getenv("SENTRY_DSN") == "" {
		log.Println("Warning: SENTRY_DSN environment variable not set. Can not report to Sentry")
	}

	err := sentry.Init(sentry.ClientOptions{
		Environment: defaultEnvironment,
	})
	if err != nil {
		log.Fatalf("Error initialising sentry: %v", err)
	}

	clientset, err := createKubernetesClient(*configFlag)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Error creating kubernetes client: %v", err)
	}

	app := application{
		clientset:          clientset,
		defaultEnvironment: os.Getenv("SENTRY_ENVIRONMENT"),
	}

	inNamespace := strings.Split(os.Getenv("NAMESPACE"), ",")
	exNamespace := strings.Split(os.Getenv("EXCLUDE_NAMESPACE"), ",")
	allNamespace := []string{v1.NamespaceAll}

	switch len(inNamespace) {
	// include all namespaces
	case 0:
		switch len(exNamespace) {
		// exclude nothing
		case 0:
			app.namespaces = allNamespace
		// exclude some
		default:
			app.namespaces = difference(allNamespace, exNamespace)
		}
	// include only some namespaces
	default:
		switch len(exNamespace) {
		// include some, exclude nothing
		case 0:
			app.namespaces = inNamespace
		// include some, exclude some from it
		default:
			app.namespaces = difference(inNamespace, exNamespace)
		}
	}

	stopSignal, err := app.Run()
	if err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Error starting monitors: %v", err)
	}
	abortSignal := make(chan os.Signal, 2)
	signal.Notify(abortSignal, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	<-abortSignal

	stopSignal <- struct{}{}
	log.Println("Exiting")
	// Make sure all events are flushed before we terminate
	sentry.Flush(time.Second * 1)
}

func createKubernetesClient(configFile string) (client *kubernetes.Clientset, err error) {
	var config *rest.Config
	if configFile == "" && !inCluster() {
		// If we are not running in a cluster default to reading ~/.kube/config
		if usr, err := user.Current(); err == nil {
			configFile = filepath.Join(usr.HomeDir, ".kube", "config")
		}
	}

	if configFile == "" {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", configFile)
	}
	if err != nil {
		return
	}
	return kubernetes.NewForConfig(config)
}

func difference(allNamespace, exNamespace []string) []string {
	mapdiff := make(map[string]struct{}, len(exNamespace))
	for _, ns := range exNamespace {
		mapdiff[ns] = struct{}{}
	}
	var diff []string
	for _, ns := range allNamespace {
		if _, found := mapdiff[ns]; !found {
			diff = append(diff, ns)
		}
	}
	return diff
}
