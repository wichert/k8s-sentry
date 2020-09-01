# Deployment

The manifests in this folder show how to deploy k8s-sentry to monitor your
Kubernetes cluster.

There are two sets of manifests:

* [all-namespaces](all-namespaces/) configures k8s-sentry to monitor all
  namespaces in the cluster.
* [single-namespace](single-namespace/) only monitors the namespace in which
  k8s-sentry is deployed.

multiple-namespaces can be monitored by providing the namespaces as comma separated 
environment variable NAMESPACE in [single-namespace](single-namespace/) manifests.

Once you have decided which set of manifests you need to use you need to do
edit its `deployment.yaml` file and insert the DSN for your Sentry project.
After you have done that you can deploy them using `kubectl apply`:

```shell
kubectl apply -f deploy/all-namespaces
```
