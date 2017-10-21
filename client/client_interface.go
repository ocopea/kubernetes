// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
// Package client provides a super thin k8s client for ocopea that covers only the minimal requirements
// At this stage the official "supported" k8s go library is shit. it pulls dependencies of the entire k8s repository(!)
// K8S maintainers recommended not using it at this stage in some group discussion
// In the future we should consider using an official k8s go client library if available
// In order to maintain compatibility in case we'll switch in the future, we're using the swagger api generated code
// Implementation is using basic http client
package client

import (
	"ocopea/kubernetes/client/types"
	"ocopea/kubernetes/client/v1"
	"time"
)

type LogMessageConsumer interface {
	Consume(message string)
}
type CloseHandle func()

type ClientInterface interface {
	CreateNamespace(ns *v1.Namespace, force bool) (*v1.Namespace, error)
	CreateReplicationController(rc *v1.ReplicationController, force bool) (*v1.ReplicationController, error)
	CheckServiceExists(serviceName string) (bool, error)
	CreateService(svc *v1.Service, force bool) (*v1.Service, error)
	CreatePersistentVolume(pv *v1.PersistentVolume, force bool) (*v1.PersistentVolume, error)
	ListPodsInfo(labelFilters map[string]string) ([]*v1.Pod, error)
	ListEntityEvents(entityUid types.UID) ([]*v1.Event, error)
	ListServiceInfo(labelFilters map[string]string) ([]*v1.Service, error)
	ListNamespaceInfo(labelFilters map[string]string) ([]*v1.Namespace, error)
	GetServiceInfo(serviceName string) (*v1.Service, error)
	GetPersistentVolumeInfo(persistentVolumeName string) (*v1.PersistentVolume, error)
	GetReplicationControllerInfo(rcName string) (*v1.ReplicationController, error)
	GetPodInfo(podName string) (*v1.Pod, error)
	GetPodLogs(podName string) ([]byte, error)
	FollowPodLogs(podName string, consumerChannel chan string) (CloseHandle, error)
	DeletePod(podName string) (*v1.Pod, error)
	CheckNamespaceExist(nsName string) (bool, error)
	DeleteNamespaceAndWaitForTermination(nsName string, maxRetries int, sleepDuration time.Duration) error
	DeleteNamespace(nsName string) error
	DeleteReplicationController(rcName string) error
	DeleteService(serviceName string) error
	RunOneOffTask(name string, containerName string, additionalVars []v1.EnvVar) error
	CreatePod(pod *v1.Pod, force bool) (*v1.Pod, error)
	TestVolume(volumeName string) (bool, *v1.PersistentVolume, error)
	TestService(serviceName string) (bool, *v1.Service, error)
	WaitForServiceToStart(serviceName string, maxRetries int, sleepDuration time.Duration) (*v1.Service, error)
	DeployReplicationController(serviceName string, rc *v1.ReplicationController, force bool) (*v1.ReplicationController, error)
}
