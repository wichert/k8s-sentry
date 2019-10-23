# crypho.com/k8s-sentry

*k8s-sentry* is a simple tool to monitor a [Kubernetes](https://kubernetes.io) cluster and report all operational issues to [Sentry](http://sentry.io).

There are two alternatives implementations:

* [getsentry/sentry-kubernetes](https://github.com/getsentry/sentry-kubernetes): The official Sentry kubernetes reporter. This is not actively maintained and suffers from a [major memory leak](https://github.com/getsentry/sentry-kubernetes/issues/7).
* [stevelacy/go-sentry-kubernetes](https://github.com/stevelacy/go-sentry-kubernetes): An alternative go implementation. This works well, but includes very little information in Sentry reports.

## Deployment

See [deploy](deploy/) for Kubernetes manifests.

## Configuration

Configuration is done completely via environment variables.

| Variable | Description |
| -- | -- |
| `SENTRY_DSN` | **Required** DSN for a Sentry project. |
| `NAMESPACE` | If set only monitor events within this Kubernetes namespace. If not set all namespaces are monitored (as far as permissions allowed) |
| `ENVIRONMENT` | Environment for Sentry issues. If not set the namespace is used as environment. |

## Building

This project uses [Go modules](https://github.com/golang/go/wiki/Modules) and requires Go 1.13 or later. From a git checkout you can build the binary using `go build`:

```shell
$ go build
go: downloading k8s.io/apimachinery v0.0.0-20191020214737-6c8691705fc5
go: downloading k8s.io/client-go v0.0.0-20191016111102-bec269661e48
go: downloading k8s.io/api v0.0.0-20191016110408-35e52d86657a
...
```

You can then run `k8s-sentry` directly:

```shell
$ ./k8s-sentry
2019/10/22 15:55:41 Warning: DSN environment variable not set. Can not report to Sentry
2019/10/22 15:55:41 Warning HorizontalPodAutoscaler/istio-ingressgateway: unable to get metrics for resource cpu: no metrics returned from resource metrics API
2019/10/22 15:55:41 Warning HorizontalPodAutoscaler/istio-pilot: unable to get metrics for resource cpu: no metrics returned from resource metrics API
```
