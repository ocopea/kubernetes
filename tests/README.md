# Ocopea Kubernetes Extension Tests

## Description

This project contains all component and integration tests for the Ocopea Kubernetes extension.

It includes 3 test runners:

1. Kubernetes Deployer Tester - end-to-end integration test for the entire Kubernetes deployment
2. Mongo DSB Tester - component test for the mongodsb service
3. Kubernetes PSB Tester - component test for the k8spsb service

 
## How to use
In order to run the tests the `tests` maven project needs to be built.
The `tests` project requires maven and to build it simply build the root maven project.

```
cd kubernetes/tests
mvn clean install
```


### Kubernetes Deployer Tester

In order to run the Kubernetes end-to-end deployment tester, run the deployer and pass the root URL of the orcs 
component deployed on Kubernetes. The deployer's `deploy-site` command prints the orcs component or you can use `kubectl` commands
to locate the orcs service endpoint.
 
```
java -jar k8s-deployer-tester/target/k8s-deployer-tester.jar http://{orcs component url}
```

In case you are using `minikube` to run the tests locally (recommended), you can use the `testKubernetesMinikube.sh` script.


### Mongo DSB Tester

In order to run the mongodsb component tester, run the deployer with the `deploy-mongodsb` command argument. This will deploy
the mongodsb component by itself and create a public service so it can be accessed outside the cluster.
The `deploy-mongodsb` command prints the DSB endpoint to be used, and it can also be found using `kubectl` commands.
 
```
java -jar mongodsb-tester/target/mongodsb-tester.jar http://{mongodsb component url}
```

In case you are using `minikube` to run the tests locally (recommended), you can use the `testMongoDsbMinikube.sh` script.
The script assumes the DSB is running in a namespace called `testing`.

### Kubernetes PSB Tester

In order to run the k8spsb component tester, run the deployer with the `deploy-k8spsb` command argument. This will deploy
the k8spsb component by itself and create a public service so it can be accessed outside the cluster.
The deploy-k8spsb command prints the DSB endpoint to be used, and it can also be found using `kubectl` commands.
 
```
java -jar k8spsb-tester/target/k8spsb-tester.jar http://{k8spsb component url}
```

In case you are using `minikube` to run the tests locally (recommended), you can use the `testK8sPsbMinikube.sh` script.
The script assumes the PSB is running in a namespace called `testing`.

**NOTE**: 
In order to run multiple tests on the same Kubernetes cluster, it is easy to deploy the tested component to a 
separate namespace (e.g. named `testing`). To do that use the `-namespace` flag with the deployer commands.
