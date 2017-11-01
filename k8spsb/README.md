# K8S PSB

This package implements Ocopea PaaS Broker (PSB) for running apps on top of Kubernetes.

For more info visit the [Ocopea website](https://ocopea.github.io).

# How to build

* Clone the kubernetes repo under GOPATH/src/ocopea/
* Run the `buildImage.sh` script from the k8spsb folder

# How to run

K8spsb is automatically deployed every time you run the 
[Kubernetes deployer](https://github.com/ocopea/kubernetes/tree/master/deployer)

For purposes of running integration tests, it is possible to deploy k8spsb as a
standalone deployment using the `deploy-k8spsb` command.
When doing so, it is recommended to deploy to a separate namespace.
For example, to run on minikube in a "testing" namespace:

```
$ go run deployer.go deploy-k8spsb -namespace=testing -local-cluster-ip=$(minikube ip)
```

# Tests

In order to run the unit tests simply use:

```
$ go test
```

To run the end-to-end and functional tests, see the 
[Ocopea Kubernetes testing project](https://github.com/ocopea/kubernetes/tree/master/tests).

