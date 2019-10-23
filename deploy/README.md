# Deployment

The manifests in this folder show how to deploy k8s-sentry to monitor your
Kubernetes cluster.

When deploying this in your cluster you need to take several steps. First
you need to setup a ServiceAccount that k8s-sentry can use. There are two
versions of that provided: `serviceaccount-all-namespaces.yaml` defines a
ServiceAccount that can monitor an entire cluster. If you only want to monitor
a single namespace you can use  `serviceaccount-single-namespace.yaml` instead.

```shell
kubectl apply -f serviceaccount-all-namespaces.yaml
```

Next you need to modify `deployment.yaml` and insert the DSN from your Sentry project.
Once you have done that you can create the deployment:

```shell
kubectl apply -f deployment.yaml
```
