// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package configuration

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type PersistentSchedulerConfiguration struct {
	Name           string `json:"name"`
	DatasourceName string `json:"datasourceName"`
	PersistTasks   string `json:"persistTasks"`
}

type DevBlobstoreConfiguration struct {
	Name string `json:"name"`
}

type StandalonePostgresDatasourceConfiguration struct {
	Server         string `json:"server"`
	Port           string `json:"port"`
	DatabaseName   string `json:"databaseName"`
	DbSchema       string `json:"dbSchema"`
	MaxConnections string `json:"maxConnections"`
	DbUser         string `json:"dbUser"`
	DbPassword     string `json:"dbPassword"`
}

type InputQueueConfig struct {
	NumberOfConsumers int      `json:"numberOfConsumers"`
	LogInDebug        bool     `json:"logInDebug"`
	DeadLetterQueues  []string `json:"deadLetterQueues"`
}

type DestinationQueueConfig struct {
	BlobstoreNameSpace     string `json:"blobstoreNameSpace"`
	BlobstoreKeyHeaderName string `json:"blobstoreKeyHeaderName"`
	LogInDebug             bool   `json:"logInDebug"`
}

type DataSourceConfig struct {
	MaxConnections int               `json:"maxConnections"`
	Properties     map[string]string `json:"properties"`
}

type DevQueueConfiguration struct {
	DestinationType string            `json:"destinationType"`
	Properties      map[string]string `json:"properties"`
}

type StaticConfigurationNode struct {
	Data     interface{}                        `json:"data"`
	Children map[string]StaticConfigurationNode `json:"children"`
}

type PersistentMessagingConfiguration struct {
	DatasourceName string `json:"datasourceName"`
	PersistMessage string `json:"persistMessage"`
}

type PersistentQueueConfiguration struct {
	DestinationType                     string `json:"destinationType"`
	QueueName                           string `json:"queueName"`
	MemoryBufferMaxMessages             string `json:"memoryBufferMaxMessages"`
	SecondsToSleepBetweenMessageRetries string `json:"secondsToSleepBetweenMessageRetries"`
	MaxRetries                          string `json:"maxRetries"`
}

type UndertowWebServerConfiguration struct {
	Port     string `json:"port"`
	BasePath string `json:"basePath"`
}

type ServiceConfig struct {
	ServiceURI               string                            `json:"serviceURI"`
	Route                    string                            `json:"route"`
	GlobalLoggingConfig      string                            `json:"globalLoggingConfig"`
	CorrelationLoggingConfig map[string]string                 `json:"correlationLoggingConfig"`
	InputQueueConfig         map[string]InputQueueConfig       `json:"inputQueueConfig"`
	DestinationQueueConfig   map[string]DestinationQueueConfig `json:"destinationQueueConfig"`
	DataSourceConfig         map[string]DataSourceConfig       `json:"dataSourceConfig"`
	BlobstoreConfig          map[string]DataSourceConfig       `json:"blobstoreConfig"`
	Parameters               map[string]string                 `json:"parameters"`
	ExternalResourceConfig   map[string]map[string]string      `json:"externalResourceConfig"`
}

type ConfigurationClient struct {
	url        string
	httpClient http.Client
}

// Constructs a new client object
func NewConfigurationClient(url string) (*ConfigurationClient, error) {
	//todo:nasty nasty insecure connect to cluster, fix one day when this becomes a real thing - Ash Nazg
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := http.Client{Transport: tr}
	c := &ConfigurationClient{url: url, httpClient: httpClient}

	err := c.verifyConnectivity(c.url)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *ConfigurationClient) verifyConnectivity(serviceURI string) error {
	fmt.Printf("testing configuration service at %s", c.url)
	resp, err := c.httpClient.Get(c.url + "/state")
	if err != nil {
		return fmt.Errorf("failed connecting to configuration server at %s - %s", c.url, err.Error())
	}
	defer resp.Body.Close()

	all, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed parsing configuration service state  at %s - %s", c.url, err.Error())
	}
	fmt.Println(string(all))

	var dat map[string]interface{}
	json.Unmarshal([]byte(all), &dat)

	state := dat["state"].(string)
	fmt.Println(state)
	if strings.Compare(state, "RUNNING") != 0 {
		return fmt.Errorf("configuration service at %s is not running but at state %s", c.url, state)
	}
	return nil
}

func (c *ConfigurationClient) RegisterService(serviceURI string, serviceConfig *ServiceConfig, overwrite bool) error {
	var method string
	if overwrite {
		method = "PUT"
	} else {
		method = "POST"
	}
	resource := c.url + "/configurations/serviceconfig/" + serviceURI

	//todo:is this the right/best/easiest way to do streaming for posting json
	b := new(bytes.Buffer)
	enc := json.NewEncoder(b)
	err := enc.Encode(*serviceConfig)
	if err != nil {
		return fmt.Errorf("Failed encoding ServiceConfig info %s", err.Error())
	}

	req, err := http.NewRequest(method, resource, b)
	if err != nil {
		return fmt.Errorf("failed creating %s request on %s - %s", method, resource, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed executing %s request on %s - %s", method, resource, err.Error())
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad respons from %s request on %s - %s", method, resource, resp.Status)
	}
	return nil
}

func (c *ConfigurationClient) RegisterScheduler(schedulerName string, dataSourceName string, persistMessages bool, overwrite bool) error {
	var method string
	if overwrite {
		method = "PUT"
	} else {
		method = "POST"
	}
	resource := c.url + "/configurations/scheduler/" + schedulerName

	var schedulerConfig *PersistentSchedulerConfiguration
	if persistMessages {
		schedulerConfig = &PersistentSchedulerConfiguration{
			Name:           schedulerName,
			DatasourceName: dataSourceName,
			PersistTasks:   "true",
		}
	} else {
		schedulerConfig = &PersistentSchedulerConfiguration{
			Name:         schedulerName,
			PersistTasks: "false",
		}

	}

	//todo:is this the right/best/easiest way to do streaming for posting json
	b := new(bytes.Buffer)
	enc := json.NewEncoder(b)
	err := enc.Encode(*schedulerConfig)
	if err != nil {
		return fmt.Errorf("Failed encoding PersistentSchedulerConfiguration info %s", err.Error())
	}

	req, err := http.NewRequest(method, resource, b)
	if err != nil {
		return fmt.Errorf("failed creating %s request on %s - %s", method, resource, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed executing %s request on %s - %s", method, resource, err.Error())
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad respons from %s request on %s - %s", method, resource, resp.Status)
	}
	return nil
}

func (c *ConfigurationClient) RegisterMessagingSystem(overwrite bool) error {
	var method string
	if overwrite {
		method = "PUT"
	} else {
		method = "POST"
	}
	resource := c.url + "/configurations/messaging/default-messaging"

	req, err := http.NewRequest(method, resource, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("failed creating %s request on %s - %s", method, resource, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed executing %s request on %s - %s", method, resource, err.Error())
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad respons from %s request on %s - %s", method, resource, resp.Status)
	}

	err = c.RegisterBlobStore("dev-messaging", overwrite)

	return nil

}
func (c *ConfigurationClient) RegisterExternalResource(resourceName string, overwrite bool) error {
	var method string
	if overwrite {
		method = "PUT"
	} else {
		method = "POST"
	}
	resource := c.url + "/configurations/external/" + resourceName

	req, err := http.NewRequest(method, resource, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("failed creating %s request on %s - %s", method, resource, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed executing %s request on %s - %s", method, resource, err.Error())
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad respons from %s request on %s - %s", method, resource, resp.Status)
	}

	return nil

}

func (c *ConfigurationClient) RegisterBlobStore(blobStoreName string, overwrite bool) error {
	var method string
	if overwrite {
		method = "PUT"
	} else {
		method = "POST"
	}
	resource := c.url + "/configurations/blobstore/" + blobStoreName

	blobStoreConfig := &DevBlobstoreConfiguration{Name: blobStoreName}
	//todo:is this the right/best/easiest way to do streaming for posting json
	b := new(bytes.Buffer)
	enc := json.NewEncoder(b)
	err := enc.Encode(*blobStoreConfig)
	if err != nil {
		return fmt.Errorf("Failed encoding DevBlobstoreConfiguration info %s", err.Error())
	}

	req, err := http.NewRequest(method, resource, b)
	if err != nil {
		return fmt.Errorf("failed creating %s request on %s - %s", method, resource, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed executing %s request on %s - %s", method, resource, err.Error())
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad respons from %s request on %s - %s", method, resource, resp.Status)
	}
	return nil
}

func (c *ConfigurationClient) RegisterQueue(queueName string, overwrite bool) error {
	var method string
	if overwrite {
		method = "PUT"
	} else {
		method = "POST"
	}
	resource := c.url + "/configurations/queue/" + queueName

	blobStoreConfig := &DevQueueConfiguration{DestinationType: "QUEUE"}
	//todo:is this the right/best/easiest way to do streaming for posting json
	b := new(bytes.Buffer)
	enc := json.NewEncoder(b)
	err := enc.Encode(*blobStoreConfig)
	if err != nil {
		return fmt.Errorf("Failed encoding DevQueueConfiguration info %s", err.Error())
	}

	req, err := http.NewRequest(method, resource, b)
	if err != nil {
		return fmt.Errorf("failed creating %s request on %s - %s", method, resource, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed executing %s request on %s - %s", method, resource, err.Error())
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad respons from %s request on %s - %s", method, resource, resp.Status)
	}
	return nil
}

func (c *ConfigurationClient) GetServiceConfig(serviceUri string) (*ServiceConfig, error) {
	method := "GET"
	resource := c.url + "/configurations/serviceconfig/" + serviceUri

	req, err := http.NewRequest(method, resource, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating %s request on %s - %s", method, resource, err.Error())
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed getting service configuration %s - %s", serviceUri, err.Error())
	}
	defer resp.Body.Close()
	var respServiceConfig ServiceConfig
	dec := json.NewDecoder(resp.Body)
	dec.Decode(&respServiceConfig)

	//todo: only do on debug
	str, err := json.Marshal(respServiceConfig)
	if err != nil {
		log.Println(err)
	}
	log.Printf("\n%s on %s returned\n\n%s\n\n", method, resource, string(str))

	return &respServiceConfig, nil

}
