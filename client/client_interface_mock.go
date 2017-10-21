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

type ClientMock struct {
	delegate *ClientInterface

	MockCreateNamespace func (ns *v1.Namespace, force bool) (*v1.Namespace, error)
	MockCreateReplicationController func (rc *v1.ReplicationController, force bool) (*v1.ReplicationController, error)
	MockCheckServiceExists func (serviceName string) (bool, error)
	MockCreateService func (svc *v1.Service, force bool) (*v1.Service, error)
	MockCreatePersistentVolume func (pv *v1.PersistentVolume, force bool) (*v1.PersistentVolume, error)
	MockListPodsInfo func (labelFilters map[string]string) ([]*v1.Pod, error)
	MockListEntityEvents func (entityUid types.UID) ([]*v1.Event, error)
	MockListServiceInfo func (labelFilters map[string]string) ([]*v1.Service, error)
	MockListNamespaceInfo func (labelFilters map[string]string) ([]*v1.Namespace, error)
	MockGetServiceInfo func (serviceName string) (*v1.Service, error)
	MockGetPersistentVolumeInfo func (persistentVolumeName string) (*v1.PersistentVolume, error)
	MockGetReplicationControllerInfo func (rcName string) (*v1.ReplicationController, error)
	MockGetPodInfo func (podName string) (*v1.Pod, error)
	MockGetPodLogs func (podName string) ([]byte, error)
	MockFollowPodLogs func (podName string, consumerChannel chan string) (CloseHandle, error)
	MockDeletePod func (podName string) (*v1.Pod, error)
	MockCheckNamespaceExist func (nsName string) (bool, error)
	MockDeleteNamespaceAndWaitForTermination func (nsName string, maxRetries int, sleepDuration time.Duration) error
	MockDeleteNamespace func (nsName string) error
	MockDeleteReplicationController func (rcName string) error
	MockDeleteService func (serviceName string) error
	MockRunOneOffTask func (name string, containerName string, additionalVars []v1.EnvVar) error
	MockCreatePod func (pod *v1.Pod, force bool) (*v1.Pod, error)
	MockTestVolume func (volumeName string) (bool, *v1.PersistentVolume, error)
	MockTestService func (serviceName string) (bool, *v1.Service, error)
	MockWaitForServiceToStart func (serviceName string, maxRetries int, sleepDuration time.Duration) (*v1.Service, error)
	MockDeployReplicationController func (serviceName string, rc *v1.ReplicationController, force bool) (*v1.ReplicationController, error)
}

func (mc *ClientMock) CreateNamespace(ns *v1.Namespace, force bool) (*v1.Namespace, error) {
	return mc.MockCreateNamespace(ns, force)
}

func (mc *ClientMock) CreateReplicationController(rc *v1.ReplicationController, force bool) (*v1.ReplicationController, error){
	return mc.MockCreateReplicationController(rc, force);
}

func (mc *ClientMock) CheckServiceExists(serviceName string) (bool, error){
	return mc.MockCheckServiceExists(serviceName)
}

func (mc *ClientMock) CreateService(svc *v1.Service, force bool) (*v1.Service, error){
	return mc.MockCreateService(svc, force)
}
func (mc *ClientMock) CreatePersistentVolume(pv *v1.PersistentVolume, force bool) (*v1.PersistentVolume, error){
	return mc.MockCreatePersistentVolume(pv, force)
}
func (mc *ClientMock) ListPodsInfo(labelFilters map[string]string) ([]*v1.Pod, error){
	return mc.MockListPodsInfo(labelFilters)
}
func (mc *ClientMock) ListEntityEvents(entityUid types.UID) ([]*v1.Event, error){
	return mc.MockListEntityEvents(entityUid)
}
func (mc *ClientMock) ListServiceInfo(labelFilters map[string]string) ([]*v1.Service, error){
	return mc.MockListServiceInfo(labelFilters)
}
func (mc *ClientMock) ListNamespaceInfo(labelFilters map[string]string) ([]*v1.Namespace, error){
	return mc.MockListNamespaceInfo(labelFilters)
}
func (mc *ClientMock) GetServiceInfo(serviceName string) (*v1.Service, error){
	return mc.MockGetServiceInfo(serviceName)
}
func (mc *ClientMock) GetPersistentVolumeInfo(persistentVolumeName string) (*v1.PersistentVolume, error){
	return mc.MockGetPersistentVolumeInfo(persistentVolumeName)
}
func (mc *ClientMock) GetReplicationControllerInfo(rcName string) (*v1.ReplicationController, error){
	return mc.MockGetReplicationControllerInfo(rcName)
}
func (mc *ClientMock) GetPodInfo(podName string) (*v1.Pod, error){
	return mc.MockGetPodInfo(podName)
}
func (mc *ClientMock) GetPodLogs(podName string) ([]byte, error) {
	return mc.MockGetPodLogs(podName)
}
func (mc *ClientMock) FollowPodLogs(podName string, consumerChannel chan string) (CloseHandle, error){
	return mc.MockFollowPodLogs(podName, consumerChannel)
}
func (mc *ClientMock) DeletePod(podName string) (*v1.Pod, error){
	return mc.MockDeletePod(podName)
}
func (mc *ClientMock) CheckNamespaceExist(nsName string) (bool, error){
	return mc.MockCheckNamespaceExist(nsName)
}
func (mc *ClientMock) DeleteNamespaceAndWaitForTermination(nsName string, maxRetries int, sleepDuration time.Duration) error{
	return mc.MockDeleteNamespaceAndWaitForTermination(nsName, maxRetries, sleepDuration)
}
func (mc *ClientMock) DeleteNamespace(nsName string) error{
	return mc.MockDeleteNamespace(nsName)
}
func (mc *ClientMock) DeleteReplicationController(rcName string) error{
	return mc.MockDeleteReplicationController(rcName)
}
func (mc *ClientMock) DeleteService(serviceName string) error{
	return mc.MockDeleteService(serviceName)
}
func (mc *ClientMock) RunOneOffTask(name string, containerName string, additionalVars []v1.EnvVar) error{
	return mc.MockRunOneOffTask(name, containerName, additionalVars)
}
func (mc *ClientMock) CreatePod(pod *v1.Pod, force bool) (*v1.Pod, error){
	return mc.MockCreatePod(pod, force)
}
func (mc *ClientMock) TestVolume(volumeName string) (bool, *v1.PersistentVolume, error){
	return mc.MockTestVolume(volumeName)
}
func (mc *ClientMock) TestService(serviceName string) (bool, *v1.Service, error){
	return mc.MockTestService(serviceName)
}
func (mc *ClientMock) WaitForServiceToStart(serviceName string, maxRetries int, sleepDuration time.Duration) (*v1.Service, error){
	return mc.MockWaitForServiceToStart(serviceName, maxRetries, sleepDuration)
}
func (mc *ClientMock) DeployReplicationController(serviceName string, rc *v1.ReplicationController, force bool) (*v1.ReplicationController, error){
	return mc.MockDeployReplicationController(serviceName, rc, force)
}

