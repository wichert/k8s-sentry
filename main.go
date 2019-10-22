package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

var configFlag = flag.String("kubeconfig", "", "Configuration file")

func main() {
	flag.Parse()

	if os.Getenv("DSN") == "" {
		log.Println("Warning: DSN environment variable not set. Can not report to Sentry")
	}

	err := sentry.Init(sentry.ClientOptions{
		Environment: os.Getenv("ENVIRONMENT"),
	})
	if err != nil {
		log.Fatalf("Error initialising sentry: %v", err)
	}

	clientset, err := createKubernetesClient(*configFlag)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatalf("Error creating kubernetes client: %v", err)
	}

	stopSignal := watchForEvents(clientset, os.Getenv("NAMESPACE"))

	abortSignal := make(chan os.Signal)
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

func watchForEvents(clientset *kubernetes.Clientset, namespace string) chan struct{} {
	if namespace == "" {
		namespace = v1.NamespaceAll
	}

	watchList := cache.NewListWatchFromClient(
		clientset.CoreV1().RESTClient(),
		"events",
		namespace,
		fields.Everything(),
	)
	_, controller := cache.NewInformer(
		watchList,
		&v1.Event{},
		time.Second*30,
		cache.ResourceEventHandlerFuncs{
			AddFunc: handleEventAdd,
		},
	)

	stop := make(chan struct{})
	go controller.Run(stop)
	return stop
}

func handleEventAdd(obj interface{}) {
	evt, ok := obj.(*v1.Event)
	if !ok {
		sentry.CaptureMessage("Unexpected event type")
		return
	}

	if evt.Type == v1.EventTypeNormal {
		return
	}

	sentryEvent := sentry.NewEvent()
	// sentryEvent.Environment = evt.Namespace
	sentryEvent.Message = fmt.Sprintf("%s/%s: %s", evt.InvolvedObject.Kind, evt.InvolvedObject.Name, evt.Message)
	sentryEvent.Level = getSentryLevel(evt)
	sentryEvent.Timestamp = evt.EventTime.Unix()
	sentryEvent.Fingerprint = getEventFingerprint(evt)
	sentryEvent.Tags["namespace"] = evt.Namespace
	sentryEvent.Tags["component"] = evt.Source.Component
	sentryEvent.Tags["cluster"] = evt.ClusterName
	sentryEvent.Tags["reason"] = evt.Reason
	sentryEvent.Tags["kind"] = evt.Kind
	sentryEvent.Tags["type"] = evt.Type
	sentryEvent.Sdk = sentry.SdkInfo{
		Name:    "crypho.com/k8s-sentry",
		Version: "1.0",
	}

	if evt.Action != "" {
		sentryEvent.Extra["action"] = evt.Action
	}
	sentryEvent.Extra["count"] = evt.Count

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

func getEventFingerprint(evt *v1.Event) []string {
	return []string{
		evt.GetClusterName(),
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
