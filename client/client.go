// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
// Package client provides a super thin k8s client for ocopea that covers only the minimal requirements
// At this stage the official "supported" k8s go library pulls dependencies of the entire k8s repository(!)
// K8S maintainers recommended not using it at this stage in some group discussion
// In the future we should consider using an official k8s go client library if available
// In order to maintain compatibility in case we'll switch in the future, we're using the swagger api generated code
// Implementation is using basic http client
package client

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"ocopea/kubernetes/client/types"
	"ocopea/kubernetes/client/v1"
	"time"
)

type Client struct {
	Url        string
	Namespace  string
	httpClient http.Client
	SslToken   string
	UserName   string
	Password   string
}

// Constructs a new client object
func NewClient(url string, namespace string, userName string, password string, certificatePath string) (*Client, error) {
	//todo:nasty nasty insecure connect to cluster, fix one day when this becomes a real thing - Ash Nazg
	log.Println(url)

	var caCertPool *x509.CertPool = nil
	log.Println("here1")
	sslToken := ""
	if certificatePath != "" {
		log.Println("here2")

		caCert, err := ioutil.ReadFile(certificatePath)
		sslToken = string(caCert)
		log.Println(sslToken)

		if err != nil {
			return nil, err
		}
		caCertPool = x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	httpClient := http.Client{Transport: tr}
	c := &Client{Url: url, Namespace: namespace, httpClient: httpClient, UserName: userName, Password: password, SslToken: sslToken}

	r, err := c.doHttpNoNS("GET", "", nil)
	if err != nil {
		return nil, err
	}

	defer r.Body.Close()
	if r.StatusCode == 200 {
		return c, nil
	} else {
		return nil, errors.New(fmt.Sprintf("Failed testing k8s connection, received status %s", r.Status))
	}
}

func (c *Client) CreateNamespace(ns *v1.Namespace, force bool) (*v1.Namespace, error) {
	respNs := &v1.Namespace{}
	err := c.createEntity("namespaces", ns.Name, ns, respNs, force)
	return respNs, err
}

func (c *Client) CreateReplicationController(rc *v1.ReplicationController, force bool) (*v1.ReplicationController, error) {
	respRc := &v1.ReplicationController{}
	err := c.createEntity("replicationcontrollers", rc.Name, rc, respRc, force)
	return respRc, err
}

func (c *Client) structToReader(s interface{}) (io.Reader, error) {
	b := new(bytes.Buffer)
	enc := json.NewEncoder(b)
	// Encoding
	//todo:go: should it be "go enc.Encode" if buff fills allow streaming?
	err := enc.Encode(s)
	return b, err

}

func (c *Client) CheckServiceExists(serviceName string) (bool, error) {
	resp, err := c.doHttp("GET", "services/"+serviceName, nil)
	if err != nil {
		return false, fmt.Errorf("Failed getting k8s service info for service %s - %s", serviceName, err.Error())
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	} else if resp.StatusCode == http.StatusOK {
		return true, nil
	} else {
		log.Printf("Unexpected response status when checking service %s - %s", serviceName, resp.Status)
		return false, fmt.Errorf("Unexpected response status when checking service %s - %s", serviceName, resp.Status)
	}
}

func isEntityTypeNamespaceLevel(entityTypeName string) bool {
	switch entityTypeName {
	case "namespaces":
		fallthrough
	case "persistentvolumes":
		return false
	default:
		return true
	}
}

func (c *Client) createEntity(entityTypeName string, entityName string, entityToCreatePtr interface{}, responseEntityPtr interface{}, force bool) error {
	httpMethod := "POST"
	resourceName := entityTypeName + "/" + entityName

	r, err := c.structToReader(entityToCreatePtr)
	if err != nil {
		return fmt.Errorf("Failed formatting entity %s to json - %s", resourceName, err.Error())
	}
	var resp *http.Response
	if isEntityTypeNamespaceLevel(entityTypeName) {
		resp, err = c.doHttp(httpMethod, entityTypeName, r)
	} else {
		resp, err = c.doHttpNoNS(httpMethod, entityTypeName, r)
	}

	if err != nil {
		return fmt.Errorf("Failed creating k8s entity %s - %s", resourceName, err.Error())
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {

		// We support create force meaning we are fine if already exist
		if resp.StatusCode == http.StatusConflict && force {
			log.Printf("conflict creating " + resourceName + ", force mode, getting info only")
			err = c.getEntityInfo(entityTypeName, entityName, responseEntityPtr)
			if err != nil {
				return fmt.Errorf("resource %s already exist but failed reading info of the existing entity - %s", resourceName, err.Error())
			}
		} else {
			defer resp.Body.Close()
			contents, _ := ioutil.ReadAll(resp.Body)
			return fmt.Errorf("http error creating %s - %s - %s", resourceName, resp.Status, contents)
		}
	} else {

		defer resp.Body.Close()

		dec := json.NewDecoder(resp.Body)
		dec.Decode(responseEntityPtr)

		//todo: only do on debug
		_, err = json.MarshalIndent(responseEntityPtr, "", "    ")
		if err != nil {
			log.Printf("Failed parsing json for response of created entity %s - %s\n", resourceName, err.Error())
		} else {
			//log.Printf("%s on %s returned\n%s\n", httpMethod, resourceName, string(str))
		}
	}

	return nil

}

func (c *Client) CreateService(svc *v1.Service, force bool) (*v1.Service, error) {
	respSvc := &v1.Service{}
	err := c.createEntity("services", svc.Name, svc, respSvc, force)
	return respSvc, err
}

func (c *Client) CreatePersistentVolume(pv *v1.PersistentVolume, force bool) (*v1.PersistentVolume, error) {
	respPv := &v1.PersistentVolume{}
	err := c.createEntity("persistentvolumes", pv.Name, pv, respPv, force)
	return respPv, err
}

func (c *Client) ListPodsInfo(labelFilters map[string]string) ([]*v1.Pod, error) {

	respPodList := &v1.PodList{}
	err := c.getEntityInfo("pods"+buildLabelsQueryString(labelFilters), "", respPodList)
	// Listing all services in namespace
	if err != nil {
		return nil, fmt.Errorf("Failed listing k8s pods - %s", err.Error())
	}
	podList := make([]*v1.Pod, 0)
	if respPodList.Items != nil {
		for i := range respPodList.Items {
			podList = append(podList, &respPodList.Items[i])
		}
	}
	return podList, nil
}

func (c *Client) ListEntityEvents(entityUid types.UID) ([]*v1.Event, error) {

	respEventList := &v1.EventList{}
	err := c.getEntityInfo(fmt.Sprintf("events?fieldSelector=involvedObject.uid=%s", entityUid), "", respEventList)

	// Listing all events for entity uid
	if err != nil {
		return nil, err
	}

	eventList := make([]*v1.Event, 0)
	if respEventList.Items != nil {
		for i := range respEventList.Items {
			eventList = append(eventList, &respEventList.Items[i])
		}
	}
	return eventList, nil
}

func buildLabelsQueryString(labelFilters map[string]string) string {
	queryString := ""
	if labelFilters != nil {
		for labelKey, labelValue := range labelFilters {
			if queryString == "" {
				queryString = "?labelSelector="
			} else {
				queryString += ","
			}

			queryString += labelKey + "%3D" + labelValue
		}
	}
	return queryString
}

func doesObjectHaveAllLabels(meta *v1.ObjectMeta, labelFilters map[string]string) bool {
	if labelFilters != nil {
		for labelKey, labelValue := range labelFilters {
			if meta.Labels[labelKey] != labelValue {
				return false
			}
		}
	}
	return true
}

func (c *Client) ListServiceInfo(labelFilters map[string]string) ([]*v1.Service, error) {
	// todo:Since api version we're using does not support filtering we do it manually for now
	respSvcList := &v1.ServiceList{}
	// Listing all services in namespace
	err := c.getEntityInfo("services", "", respSvcList)
	if err != nil {
		return nil, fmt.Errorf("Failed listing k8s services - %s", err.Error())
	}

	// Filtering
	svcList := make([]*v1.Service, 0)
	if respSvcList.Items != nil {
		for i := range respSvcList.Items {
			if doesObjectHaveAllLabels(&respSvcList.Items[i].ObjectMeta, labelFilters) {
				svcList = append(svcList, &respSvcList.Items[i])
			}
		}
	}

	return svcList, nil

}

func (c *Client) ListNamespaceInfo(labelFilters map[string]string) ([]*v1.Namespace, error) {

	respNsList := &v1.NamespaceList{}
	// Listing all services in namespace
	err := c.getEntityInfo("namespaces", "", respNsList)
	if err != nil {
		return nil, err
	}

	nscList := make([]*v1.Namespace, 0)
	if respNsList.Items != nil {
		for i := range respNsList.Items {
			if doesObjectHaveAllLabels(&respNsList.Items[i].ObjectMeta, labelFilters) {
				nscList = append(nscList, &respNsList.Items[i])
			}
		}
	}

	return nscList, nil

}

func (c *Client) GetServiceInfo(serviceName string) (*v1.Service, error) {
	svc := &v1.Service{}
	err := c.getEntityInfo("services", serviceName, svc)
	return svc, err
}

func (c *Client) GetPersistentVolumeInfo(persistentVolumeName string) (*v1.PersistentVolume, error) {
	resp, err := c.doHttpNoNS("GET", "persistentvolumes/"+persistentVolumeName, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed getting pv info for pv %s - %s", persistentVolumeName, err.Error())
	}
	var respPv v1.PersistentVolume
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&respPv)
	if err != nil {
		return nil, fmt.Errorf("Failed decoding pv %s - %s", persistentVolumeName, err.Error())
	}

	//todo: only do on debug
	str, err := json.MarshalIndent(respPv, "", "    ")
	if err != nil {
		log.Println(err)
	}
	log.Println(string(str))

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Failed getting k8s pv info for pv %s - %s", persistentVolumeName, respPv)
	}

	return &respPv, nil
}

func (c *Client) GetReplicationControllerInfo(rcName string) (*v1.ReplicationController, error) {
	rc := &v1.ReplicationController{}
	err := c.getEntityInfo("replicationcontrollers", rcName, rc)
	return rc, err
}

func (c *Client) getEntityInfo(entityTypeName string, entityName string, entityStructPtr interface{}) error {
	httpMethod := "GET"
	resourceName := entityTypeName
	if entityName != "" {
		resourceName += "/" + entityName
	}

	var err error
	var resp *http.Response
	if isEntityTypeNamespaceLevel(entityTypeName) {
		resp, err = c.doHttp(httpMethod, resourceName, nil)
	} else {
		resp, err = c.doHttpNoNS(httpMethod, resourceName, nil)
	}
	if err != nil {
		return fmt.Errorf("Failed getting k8s info for entity %s - %s", resourceName, err.Error())
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Failed getting k8s %s with code %s", resourceName, resp.Status)
	}

	dec := json.NewDecoder(resp.Body)
	dec.Decode(entityStructPtr)

	//todo: only do on debug
	str, err := json.MarshalIndent(entityStructPtr, "", "    ")
	if err != nil {
		log.Printf("Failed parsing json for response of entity %s - %s\n", resourceName, err.Error())
	} else {
		log.Printf("%s on %s returned\n%s\n", httpMethod, resourceName, string(str))
	}
	return nil

}
func (c *Client) GetPodInfo(podName string) (*v1.Pod, error) {
	pod := &v1.Pod{}
	err := c.getEntityInfo("pods", podName, pod)
	return pod, err
}

func (c *Client) GetPodLogs(podName string) ([]byte, error) {
	httpMethod := "GET"
	resourceName := "pods/" + podName + "/log?pretty=trye"

	resp, err := c.doHttp(httpMethod, resourceName, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed getting k8s %s info for service %s - %s", resourceName, podName, err.Error())
	}
	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed reeading response %s - %s: ", resourceName, err.Error())
	}

	return content, nil
}

func (c *Client) FollowPodLogs(podName string, consumerChannel chan string) (CloseHandle, error) {
	httpMethod := "GET"
	resourceName := "pods/" + podName + "/log?follow=trye"
	resp, err := c.doHttp(httpMethod, resourceName, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed following k8s logs for pod %s - %s", podName, err.Error())
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Failed following k8s logs for pod %s with status %s", podName, resp.Status)
	}
	closeChannel := make(chan bool, 1)
	reader := bufio.NewReader(resp.Body)
	log.Printf("following pod %s logs\n", podName)
	go func() {
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				log.Printf("Error reading log for pod %s - %s", podName, err.Error())
				return
			} else {
				select {
				case consumerChannel <- (string(line)):
				case _ = <-closeChannel:
					log.Printf("stopped following pod %s logs\n", podName)
				}

			}
		}
	}()

	return func() {
		log.Printf("done following pod %s logs\n", podName)
		closeChannel <- true
		resp.Body.Close()
	}, nil
}

func (c *Client) DeletePod(podName string) (*v1.Pod, error) {
	httpMethod := "DELETE"
	resourceName := "pods/" + podName
	resp, err := c.doHttp(httpMethod, resourceName, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed deleting k8s %s for %s - %s", resourceName, podName, err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Failed deleting k8s %s for %s - %s", resourceName, podName, resp.Status)
	}

	defer resp.Body.Close()
	var respPod v1.Pod
	dec := json.NewDecoder(resp.Body)
	dec.Decode(&respPod)

	//todo: only do on debug
	str, err := json.MarshalIndent(respPod, "", "    ")
	if err != nil {
		log.Println(err)
	}
	log.Printf("\n%s on %s returned\n\n%s\n\n", httpMethod, resourceName, string(str))

	return &respPod, nil
}

func (c *Client) CheckNamespaceExist(nsName string) (bool, error) {
	r, err := c.doHttpNoNS("GET", "namespaces/"+nsName, nil)
	if err != nil {
		return false, err
	}
	defer r.Body.Close()
	if r.StatusCode == 200 {
		return true, nil
	} else {
		return false, nil
	}
}
func (c *Client) DeleteNamespaceAndWaitForTermination(nsName string, maxRetries int, sleepDuration time.Duration) error {
	exist, err := c.CheckNamespaceExist(nsName)
	if err != nil {
		return err
	}
	if !exist {
		log.Printf("Could not find namespace %s when trying to delete\n", nsName)
		return nil
	}
	err = c.DeleteNamespace(nsName)
	if err != nil {
		return err
	}
	nsStillTerminating := true

	for retries := maxRetries; nsStillTerminating && retries > 0; retries-- {
		time.Sleep(sleepDuration)
		log.Printf("Waiting for namespace %s to vanish, %d/%d\n", nsName, maxRetries-retries, maxRetries)
		nsStillTerminating, err = c.CheckNamespaceExist(nsName)
		if err != nil {
			return err
		}
	}

	// If service did not start by now, fail the deployment
	if nsStillTerminating {
		return fmt.Errorf("Namespace %s failed to terminate even after %d retries", nsName, maxRetries)
	}

	return nil
}

func (c *Client) DeleteNamespace(nsName string) error {
	return c.deleteEntity("namespaces/" + nsName)
}

func (c *Client) DeleteReplicationController(rcName string) error {
	return c.deleteEntity("namespaces/" + c.Namespace + "/" + "replicationcontrollers/" + rcName)
}
func (c *Client) DeleteService(serviceName string) error {
	return c.deleteEntity("namespaces/" + c.Namespace + "/" + "services/" + serviceName)
}

func (c *Client) deleteEntity(relativeUrl string) error {
	httpMethod := "DELETE"
	resp, err := c.doHttpNoNS(httpMethod, relativeUrl, nil)
	if err != nil {
		return fmt.Errorf("Failed deleting %s - %s", relativeUrl, err.Error())
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("Failed deleting %s received status %s", relativeUrl, resp.Status)
	}
	defer resp.Body.Close()

	// Printing output
	btt, err := ioutil.ReadAll(resp.Body)
	if err == nil {
		buf := new(bytes.Buffer)
		err = json.Indent(buf, btt, "", "  ")
		if err == nil {

			// todo: log level!
			//log.Printf("delete on %s returned:\n%s", relativeUrl, buf)
		}
	}

	return nil
}

func (c *Client) doHttp(method string, resource string, r io.Reader) (*http.Response, error) {
	return c.doHttpNoNS(method, "namespaces/"+c.Namespace+"/"+resource, r)
}

func (c *Client) doHttpNoNS(method string, resource string, r io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.Url+"/api/v1/"+resource, r)
	if err != nil {
		return nil, fmt.Errorf("failed %s request on %s - %s", method, resource, err.Error())
	}

	// Setting authentication
	if len(c.SslToken) > 0 {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.SslToken))
	} else if len(c.UserName) > 0 {
		req.SetBasicAuth(c.UserName, c.Password)
	}

	req.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	log.Printf("%s on %s returned %d\n", method, resource, response.StatusCode)
	return response, err
}

func (c *Client) RunOneOffTask(name string, containerName string, additionalVars []v1.EnvVar) error {
	// Building rc spec

	pod := &v1.Pod{}
	pod.ObjectMeta = v1.ObjectMeta{Name: name}
	pod.ObjectMeta.Labels = make(map[string]string)
	pod.Labels["app"] = "bootstrap"
	pod.ObjectMeta.Labels["nazKind"] = "sys"

	containerSpec := v1.Container{}
	containerSpec.Name = "bootstrap"
	containerSpec.Image = containerName
	containerSpec.ImagePullPolicy = v1.PullIfNotPresent
	containerSpec.Ports = []v1.ContainerPort{{ContainerPort: 8000}}
	containerSpec.Env = additionalVars
	containers := []v1.Container{containerSpec}

	pod.Spec = v1.PodSpec{RestartPolicy: v1.RestartPolicyNever}
	pod.Spec.Containers = containers

	// The pod
	createdPod, err := c.CreatePod(pod, false)
	if err != nil {
		return fmt.Errorf("Failed creating task pod %s - %s", name, err.Error())
	}

	// Deleting this pod so it won't stay there forever
	defer c.DeletePod(createdPod.Name)

	// Wait until pod has been launched and finished

	for retries := 60; createdPod.Status.Phase != v1.PodSucceeded &&
		createdPod.Status.Phase != v1.PodFailed &&
		retries > 0; retries-- {
		log.Printf("Waiting task to execute... %d", retries)
		time.Sleep(3 * time.Second)
		createdPod, err = c.GetPodInfo(createdPod.Name)
		if err != nil {
			return fmt.Errorf("Failed getting task pod %s info - %s", name, err.Error())
		}
	}

	//todo:tail logs in a go routine as we go...
	podLog, err := c.GetPodLogs(createdPod.Name)
	if err != nil {
		log.Printf("Failed retreiving task pod %s logs, %s", createdPod.Name, err.Error())
	}
	fmt.Printf("Task Pod Logs\n%s", string(podLog))

	if createdPod.Status.Phase != v1.PodSucceeded &&
		createdPod.Status.Phase != v1.PodFailed {
		return fmt.Errorf("task pod %s failed to finish in a timely fashion", name)
	} else if createdPod.Status.Phase != v1.PodSucceeded {
		return fmt.Errorf("task pod %s has miserably failed", name)
	}

	return err
}

func (c *Client) CreatePod(pod *v1.Pod, force bool) (*v1.Pod, error) {
	respPod := &v1.Pod{}
	err := c.createEntity("pods", pod.Name, pod, respPod, force)
	return respPod, err
}

func (c *Client) TestVolume(volumeName string) (bool, *v1.PersistentVolume, error) {
	var pv *v1.PersistentVolume
	var err error
	pv, err = c.GetPersistentVolumeInfo(volumeName)
	if err != nil {
		return false, nil, fmt.Errorf("Failed getting k8s pv for %s - %s", volumeName, err.Error())
	}

	switch pv.Status.Phase {
	case v1.VolumePending:
		return false, pv, nil
	case v1.VolumeAvailable:
		return true, pv, nil
	case v1.VolumeBound:
		return true, pv, nil
	case v1.VolumeReleased:
		return false, pv, fmt.Errorf("volume %s is released - %s", volumeName, pv.Status.Message)
	case v1.VolumeFailed:
		return false, pv, fmt.Errorf("volume %s is failed - %s", volumeName, pv.Status.Message)
	default:
		return false, pv, fmt.Errorf("volume %s is %s - %s", volumeName, pv.Status.Phase, pv.Status.Message)
	}
}

func (c *Client) TestService(serviceName string) (bool, *v1.Service, error) {
	var svc *v1.Service
	var err error
	svc, err = c.GetServiceInfo(serviceName)
	if err != nil {
		return false, nil, fmt.Errorf("Failed getting k8s service for %s - %s", serviceName, err.Error())
	}

	if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		return len(svc.Status.LoadBalancer.Ingress) > 0 && (len(svc.Status.LoadBalancer.Ingress[0].IP) > 0 ||
				len(svc.Status.LoadBalancer.Ingress[0].Hostname) > 0),
			svc, nil
	} else if svc.Spec.Type == v1.ServiceTypeNodePort {
		return len(svc.Spec.Ports) > 0 &&
				svc.Spec.Ports[0].NodePort > 0,
			svc, nil
	} else if svc.Spec.Type == v1.ServiceTypeClusterIP {
		// todo, find how..
		time.Sleep(5 * time.Second)
		return true, svc, nil
	} else {
		return false, svc, fmt.Errorf("Unsupported k8s service type %s for service %s", svc.Spec.Type, serviceName)
	}

}
func (c *Client) WaitForServiceToStart(serviceName string, maxRetries int, sleepDuration time.Duration) (*v1.Service, error) {
	serviceReady := false

	var svc *v1.Service
	var err error
	for retries := maxRetries; !serviceReady && retries > 0; retries-- {
		if retries != maxRetries {
			time.Sleep(sleepDuration)
		}

		log.Printf("Waiting for service %s to start serving, %d/%d\n", serviceName, maxRetries-retries, maxRetries)
		serviceReady, svc, err = c.TestService(serviceName)
		if err != nil {
			return nil, fmt.Errorf("Failed getting k8s service for %s - %s", serviceName, err.Error())
		}
	}

	// If service did not start by now, fail the deployment
	if !serviceReady {
		return svc, fmt.Errorf("Service %s failed to start after %d retries", serviceName, maxRetries)
	}

	return svc, nil

}

func (c *Client) DeployReplicationController(
	serviceName string,
	rc *v1.ReplicationController,
	force bool) (*v1.ReplicationController, error) {
	rc, err := c.CreateReplicationController(rc, force)
	if err != nil {
		return nil, fmt.Errorf(
			"Failed creating k8s replication controller for %s - %s",
			serviceName,
			err.Error())
	}
	log.Printf("%s replication controller has been deployed successfully\n", rc.Name)

	// Now waiting for replication controller to schedule a single replication
	for retries := 60; rc.Status.Replicas == 0 && retries > 0; retries-- {
		rc, err = c.GetReplicationControllerInfo(rc.Name)
		if err != nil {
			return nil, fmt.Errorf(
				"Failed getting k8s replication controller for %s - %s",
				serviceName,
				err.Error())
		}
		time.Sleep(1 * time.Second)
	}

	if rc.Status.Replicas == 0 {
		return nil, fmt.Errorf(
			"Replication controller %s failed creating replicas after waiting for 60 seconds",
			rc.Name)
	}

	log.Printf(
		"%s replication controller has been deployed successfully and replicas already been observed\n",
		rc.Name)

	// Now we want to see that we have a pod scheduled by the rc
	rcPod, err := c.waitForReplicationControllerPodToSchedule(rc)
	if err != nil {
		return nil, err
	}

	log.Printf("pod %s for replication controller %s has been scheduled and observed\n", rcPod.Name, rc.Name)

	// Now we're waiting to the scheduled pod to actually start with a running container
	err = c.waitForPodToBeRunning(rcPod)
	if err != nil {
		return nil, err
	}
	log.Printf("pod %s for replication controller %s has been started successfuly\n", rcPod.Name, rc.Name)

	return rc, nil

}

func (c *Client) waitForPodToBeRunning(pod *v1.Pod) error {
	var err error = nil
	var numberOfEventsEncountered int = 0
	// now we want to see that the stupid pod is really starting!
	for retries := 900; pod.Status.Phase != "Running" && retries > 0; retries-- {
		pod, err = c.GetPodInfo(pod.Name)
		if err != nil {
			return fmt.Errorf(
				"Failed getting pod %s while waiting for it to be a sweetheart and run - %s",
				pod.Name,
				err.Error())
		}
		if pod.Status.ContainerStatuses[0].State.Waiting != nil &&
			pod.Status.ContainerStatuses[0].State.Waiting.Reason == "ErrImagePull" {
			return fmt.Errorf(
				"Pod %s failed to start. failed pulling image %s - %s",
				pod.Name,
				pod.Status.ContainerStatuses[0].Image,
				pod.Status.ContainerStatuses[0].State.Waiting.Message)

		} else if pod.Status.ContainerStatuses[0].State.Terminated != nil {
			break
		}

		// Getting pod events in order to print to console progress
		podEvents, err := c.ListEntityEvents(pod.UID)

		// In case we have an error when collecting events, skip it, we this is for logging only
		if err != nil {
			log.Printf(
				"Failed listing pod %s events while waiting for it to start, oh well - %s\n",
				pod.Name,
				err.Error())
		}

		// In case we encounter new events, we log them
		if len(podEvents) > numberOfEventsEncountered {

			// slicing and printing only newly encountered events
			for _, newEvent := range podEvents[numberOfEventsEncountered:] {
				if newEvent.Reason == "Pulling" {
					fmt.Printf(
						"pod %s is pulling an image from docker registry. "+
							"this might take a while, please be patient...\n%s\n",
						pod.Name,
						newEvent.Message)
				} else {
					fmt.Printf("pod %s: %s - %s\n", pod.Name, newEvent.Reason, newEvent.Message)
				}
			}

			// Updating events already printed to log
			numberOfEventsEncountered = len(podEvents)
		}

		time.Sleep(1 * time.Second)
	}

	if pod.Status.Phase == "Running" {
		time.Sleep(3 * time.Second)
		log.Printf("pod %s is now running, yey\n", pod.Name)
		return nil
	} else {

		additionalErrorMessage := ""
		// Enriching error message
		if pod.Status.ContainerStatuses != nil &&
			len(pod.Status.ContainerStatuses) > 0 {
			if pod.Status.ContainerStatuses[0].State.Waiting != nil {
				additionalErrorMessage +=
					fmt.Sprintf(
						". container still waiting. reason:%s; message:%s",
						pod.Status.ContainerStatuses[0].State.Waiting.Reason,
						pod.Status.ContainerStatuses[0].State.Waiting.Message)
			} else if pod.Status.ContainerStatuses[0].State.Terminated != nil {
				additionalErrorMessage +=
					fmt.Sprintf(
						". container terminated. reason:%s; message:%s; exit code:%d",
						pod.Status.ContainerStatuses[0].State.Terminated.Reason,
						pod.Status.ContainerStatuses[0].State.Terminated.Message,
						pod.Status.ContainerStatuses[0].State.Terminated.ExitCode,
					)
			}
		}
		return fmt.Errorf(
			"Pod %s did not start after 15 freakin' minutes and found in phase %s%s",
			pod.Name,
			pod.Status.Phase,
			additionalErrorMessage)
	}
}

func (c *Client) waitForReplicationControllerPodToSchedule(
	rc *v1.ReplicationController) (*v1.Pod, error) {
	log.Printf("searching for pods scheduled by replication controller %s\n", rc.Name)
	for retries := 60; retries > 0; retries-- {

		// Searching for the single pod scheduled by the rc
		rcPods, err := c.ListPodsInfo(rc.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf(
				"Failed searching for pods scheduled for rc %s - %s",
				rc.Name,
				err.Error())
		}
		if len(rcPods) == 0 {
			log.Printf("Could not yet find pods associated with replication controller %s\n", rc.Name)
		} else {
			thePod := rcPods[0]
			log.Printf("Found Pod %s, scheduled for rc %s\n", thePod.Name, rc.Name)
			return thePod, nil
		}
		time.Sleep(1 * time.Second)
	}

	return nil, fmt.Errorf(
		"Failed finding pod scheduled for replication controller %s after waiting for 60 seconds",
		rc.Name)

}
