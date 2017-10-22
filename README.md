# Ocopea Kubernetes Extension

Ocopea is a [Kubernetes](https://kubernetes.io) extension allowing developers to take application copies of 
their deployments.


## Description
Ocopea Kubernetes extension simplifies the development process of complex multi-microservice apps in multi-cluster 
environments. Untangling the complexity of orchestrating the restoration of production copies for debugging and 
automated tests are two of the most common tasks our rich API and UI offers. 

**Learn More**

* [Ocopea use cases](https://ocopea.github.io).

## Installation

All you'll have to do in order to start using Ocopea is clone this repo under your `GOPATH` and start playing.

```
$ cd $GOPATH/src
$ mkdir ocopea
$ cd ocopea
$ git clone https://github.com/ocopea/kubernetes.git
```

## Usage Instructions

Deploying Ocopea site on your Kubernetes cluster is as simple as running one command.
For simplicity, we recommend using kubectl proxy instead of passing API info for Ocopea.

```
$ kubectl proxy -p 8080
```

When deploying to an on premises cluster (local deployment), pass the Kubernetes cluster IP as the `local-cluster-ip`
Use the `site-name` flag to name the site (e.g. "Durham datacenter");
otherwise, the cluster IP will be used to identify the site.

```
$ cd deployer
$ go run deployer.go deploy-site -local-cluster-ip={k8s cluster ip} [-site-name={your site name}] 
```

When deploying to a public cloud cluster (e.g. GCE, AWS), use the `deployment-type` flag with the cloud of choice.
It is recommended to set the `site-name` flag to match the cluster name in the cloud platform
(e.g. gcloud cluster name). 

```
$ cd deployer
$ go run deployer.go deploy-site -deployment-type=gce -user={...} -password={...} -site-name=europe-west2-c1 
```

The easiest way to explore Ocopea on your laptop is by using `minikube`. 
A `minikube` deployment is no different than other local clusters.

```
$ minikube start
$ kubectl proxy -p 8080
$ cd deployer
$ go run deployer.go deploy-site -local-cluster-ip=$(minikube ip) 
```

Use the `-cleanup=true` flag if you wish to redeploy to the same site.


## Contribution

* [Contributing to Ocopea](https://github.com/ocopea/documentation/docs/contributing.md)
* [Ocopea Developer Guidelines](https://github.com/ocopea/documentation/docs/guidelines.md)

## Quality

Every pull request must pass the full tests of this repository.

1. All go code must be formatted with `gofmt`
2. All repository tests must pass. See more information regarding how to run the tests 
[here](https://github.com/ocopea/kubernetes/tree/master/tests).



## Licensing

**{code} does not provide legal guidance on which open source license should be used in projects. 
We do expect that all projects and contributions will have a valid open source license within the repo/project, 
or align to the appropriate license for the project/contribution.** The default license used for {code} Projects 
is the [MIT License](http://codedellemc.com/sampledocs/LICENSE "LICENSE").

Ocopea Kubernetes extension is freely distributed under the 
[MIT License](http://emccode.github.io/sampledocs/LICENSE "LICENSE"). See LICENSE for details.


## Support

Please file bugs and issues on the Github issues page for this project. 
This is to help keep track and document everything related to this repo. 
For general discussions and further support you can join the 
[{code} Community slack channel](http://community.codedellemc.com/). 
The code and documentation are released with no warranties or SLAs and are intended to be supported 
through a community driven process.
