# K8S PSB

This package implements Ocopea PaaS Broker (PSB) for running apps on top of Kubernetes.

For more info visit the [Ocopea website](https://ocopea.github.io).

# How to build

* clone the kubernetes repo under GOPATH/src/ocopea/
* run the `buildImage.sh` script from the k8spsb folder

# How to run

k8spsb is being automatically deployed every time you run the 
[Kubernetes deployer](https://github.com/ocopea/kubernetes)

For running integration tests purpose it is possible to deploy k8spsb as a standalone deployment using
using the `deploy-k8spsb` command.
When doing so, it is recommended to deploy to a separate namespace.
for example for running the dsb on minikube into a "testing" namespace:

```
$ go run deployer.go deploy-k8spsb -namespace=testing -local-cluster-ip=$(minikube ip)
```

# Tests

In order to run the unit tests simply use:

```
$ go test
```

to run the end-to-end and functional tests see the 
[testing project](https://github.com/ocopea/kubernetes/tree/add-psb-ut/tests)