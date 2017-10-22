#!/bin/bash
# Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
set -o xtrace
set -e
set -o pipefail

go run deployer.go deploy-site -local-cluster-ip=$(minikube ip) -cleanup=true
kubectl describe svc orcs --namespace=ocopea | grep NodePort: | awk '{print $3}' | awk -F'/' '{print "http://minikubeip:"$1"/hub-web-api/html/nui/index.html"}' | sed "s/minikubeip/$(minikube ip)/g"
