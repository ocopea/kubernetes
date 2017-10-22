// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ocopea/kubernetes/client"
	"ocopea/kubernetes/client/v1"
	"reflect"
	"testing"
)

var expectedPsbInfo = psbInfo{
	Name:                  "k8s-psb",
	Version:               "0.1",
	Type:                  "k8s",
	Description:           "Ocopea Kubernetes Paas Broker",
	AppServiceIdMaxLength: 24,
}

var expectedAppInstanceInfo = appInstanceInfo{
	Name:          "appService1",
	Status:        "running",
	Instances:     1,
	EntryPointURL: "http://10.11.12.13",
}

// PSB info call matches expectation
func TestHandlePsbInfo(t *testing.T) {

	_, err := http.NewRequest("GET", "http://psb/info", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(handlePsbInfo))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("invalid status %d, expected %d", res.StatusCode, http.StatusOK)
	}

	dec := json.NewDecoder(res.Body)
	var psbInfoResult psbInfo
	err = dec.Decode(&psbInfoResult)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if !reflect.DeepEqual(psbInfoResult, expectedPsbInfo) {
		t.Errorf("invalid psb Info returned: %v, want %v", psbInfoResult, expectedPsbInfo)
	}
}

// Testing app app service info
func TestHandleAppServiceInfo(t *testing.T) {

	parseRequestVars = func(r *http.Request) map[string]string {
		return map[string]string{
			"appServiceId": "appService1",
			"space":        "space1",
		}
	}

	kClient = &client.ClientMock{
		MockCheckServiceExists: func(serviceName string) (bool, error) {
			return true, nil
		},
		MockTestService: func(serviceName string) (bool, *v1.Service, error) {
			return true,
				&v1.Service{
					ObjectMeta: v1.ObjectMeta{
						Name: serviceName,
					},
					Spec: v1.ServiceSpec{
						Type: v1.ServiceTypeLoadBalancer,
					},
					Status: v1.ServiceStatus{
						LoadBalancer: v1.LoadBalancerStatus{
							Ingress: []v1.LoadBalancerIngress{
								{
									IP: "10.11.12.13",
								},
							},
						},
					},
				},
				nil
		},
	}

	_, err := http.NewRequest("GET", "http://psb/app-services/space1/appService1", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(appServiceInfoHandler))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("invalid status %d, expected %d", res.StatusCode, http.StatusOK)
	}

	dec := json.NewDecoder(res.Body)
	var appInstanceInfoResult appInstanceInfo
	err = dec.Decode(&appInstanceInfoResult)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if !reflect.DeepEqual(appInstanceInfoResult, expectedAppInstanceInfo) {
		t.Errorf("invalid app instance info returned: %v, want %v", appInstanceInfoResult, expectedAppInstanceInfo)
	}
}

// Testing app service info when service does not exist
func TestHandleAppServiceInfoServiceDoesNotExist(t *testing.T) {

	parseRequestVars = func(r *http.Request) map[string]string {
		return map[string]string{
			"appServiceId": "appService1",
			"space":        "space1",
		}
	}

	kClient = &client.ClientMock{
		MockCheckServiceExists: func(serviceName string) (bool, error) {
			return false, nil
		},
		MockTestService: func(serviceName string) (bool, *v1.Service, error) {
			return true,
				&v1.Service{
					ObjectMeta: v1.ObjectMeta{
						Name: serviceName,
					},
				},
				nil
		},
	}

	_, err := http.NewRequest("GET", "http://psb/app-services/space1/appService1", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(appServiceInfoHandler))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("invalid status %d, expected %d", res.StatusCode, http.StatusNotFound)
	}
}

// Testing app service info when service exist but not ready
func TestHandleAppServiceInfoServiceDoesNotReady(t *testing.T) {

	parseRequestVars = func(r *http.Request) map[string]string {
		return map[string]string{
			"appServiceId": "appService1",
			"space":        "space1",
		}
	}

	kClient = &client.ClientMock{
		MockCheckServiceExists: func(serviceName string) (bool, error) {
			return true, nil
		},
		MockTestService: func(serviceName string) (bool, *v1.Service, error) {
			return false,
				&v1.Service{
					ObjectMeta: v1.ObjectMeta{
						Name: serviceName,
					},
				},
				nil
		},
	}

	_, err := http.NewRequest("GET", "http://psb/app-services/space1/appService1", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(appServiceInfoHandler))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("invalid status %d, expected %d", res.StatusCode, http.StatusOK)
	}

	dec := json.NewDecoder(res.Body)
	var appInstanceInfoResult appInstanceInfo
	err = dec.Decode(&appInstanceInfoResult)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if appInstanceInfoResult.Status != "starting" {
		t.Errorf("invalid app instance info returned: %s, want %s", appInstanceInfoResult.Status, "running")
	}
}
