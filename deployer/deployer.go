// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	k8sClient "ocopea/kubernetes/client"
	"ocopea/kubernetes/client/types"
	"ocopea/kubernetes/client/v1"
	"ocopea/kubernetes/deployer/cmd"
	"ocopea/kubernetes/deployer/configuration"
	"os"
	"strconv"
	"strings"
	"time"
)

/**
This utility deploys ocopea site instance to a k8s cluster
It assumes all docker images are built and published to a repository accessible by the k8s cluster
*/

const OCOPEA_ADMIN_USERNAME = "admin"
const OCOPEA_ADMIN_PASSWORD = "nazgul"

type deployServiceInfo struct {
	ServiceName                string
	ImageName                  string
	EnvVars                    map[string]string
	ServiceRegistrationDetails []serviceRegistrationDetail
}

type deploySiteArgsBag struct {
	siteName           *string
	cleanup            *bool
	verboseSiteLogging *bool
}

type deployServiceArgsBag struct {
	cleanup *bool
}

var deploySiteArgs *deploySiteArgsBag
var deployMongoDsbArgs *deployServiceArgsBag
var deployK8sPsbArgs *deployServiceArgsBag

type UICommandAddDockerArtifactRegistry struct {
	SiteId   string `json:"siteId"`
	Name     string `json:"name"`
	Url      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type UISite struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Urn  string `json:"urn"`
}

type resourceLocation struct {
	Urn string `json:"urn"`
	Url string `json:"url"`
}

type UICommandAddPsb struct {
	SiteId string `json:"siteId"`
	PsbUrn string `json:"psbUrn"`
	PsbUrl string `json:"psbUrl"`
}

type UICommandAddDsb struct {
	SiteId string `json:"siteId"`
	DsbUrn string `json:"dsbUrn"`
	DsbUrl string `json:"dsbUrl"`
}

type UICommandAddCrb struct {
	SiteId string `json:"siteId"`
	CrbUrn string `json:"crbUrn"`
	CrbUrl string `json:"crbUrl"`
}

type serviceRegistrationDetail struct {
	ServiceUrn            string
	ServiceParameters     map[string]string
	InputQueues           []string
	AlternativePathSuffix string
}

func deployPostgres(ctx *cmd.DeployerContext) (*v1.Service, error) {
	// Deploy pg service
	pgSvc, err := deployService(
		ctx.Client,
		"nazdb",
		ctx.DeploymentType,
		false,
		true,
		[]v1.EnvVar{{
			Name:  "POSTGRES_PASSWORD",
			Value: "nazsecret",
		}},
		5432,
		5432,
		[]v1.ServicePort{},
		"postgres",
		"infra",
		ctx.ClusterIp)

	if err != nil {
		return nil, err
	}

	fmt.Println("postgres deployed, verifying connectivity " + pgSvc.Spec.ClusterIP)

	// verifying postgres is started
	pgService, err := ctx.Client.WaitForServiceToStart("nazdb", 100, 3*time.Second)
	if err != nil {
		return nil, err
	}
	fmt.Printf("postgres deployed at %s\n", pgService.Spec.ClusterIP)

	return pgService, nil

}

func deployOrcsService(
	ctx *cmd.DeployerContext,
	pgService *v1.Service,
) (*v1.Service, error) {

	// Creating the service descriptor
	svc, err := createK8SServiceStruct(
		"orcs",
		ctx.DeploymentType,
		true,
		80,
		8080,
		[]v1.ServicePort{})

	if err != nil {
		return nil, err
	}

	// Deploying the service onto k8s and waiting for it to start
	fmt.Println("deploying the orcs service")
	svc, err = deployK8SService(ctx.Client, svc, true)
	if err != nil {
		return nil, err
	}

	var serviceUrl string
	if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		serviceUrl =
			"http://" + extractLoadBalancerAddress(svc.Status.LoadBalancer) + ":80"
	} else if svc.Spec.Type == v1.ServiceTypeNodePort {
		serviceUrl = "http://" + ctx.ClusterIp + ":" +
			strconv.Itoa(svc.Spec.Ports[0].NodePort)
	} else {
		return nil, errors.New("Failed locating public Url when deploying orcs")
	}

	fmt.Println("orcs service deployed successfuly")
	log.Printf("orcs service deployed to %s/hub-web-api/html/ui/index.html\n", serviceUrl)

	rcRequest, err := createReplicationControllerStruct(
		"orcs",
		ctx.DeploymentType,
		"ocopea/orcs-k8s-runner",
		"orcs")

	if err != nil {
		return nil, fmt.Errorf("Failed creating replication controller for orcs - %s", err.Error())
	}

	//todo:amit: I don't think we need this in orcs container
	rcRequest.Spec.Template.Spec.Containers[0].Env =
		append(
			rcRequest.Spec.Template.Spec.Containers[0].Env,
			v1.EnvVar{Name: "K8S_USERNAME", Value: ctx.Client.UserName},
			v1.EnvVar{Name: "K8S_PASSWORD", Value: ctx.Client.Password},
			v1.EnvVar{Name: "OCOPEA_NAMESPACE", Value: ctx.Client.Namespace})

	rootConfNode := createOrcsServicesConfiguration(
		pgService,
		*deploySiteArgs.siteName,
		*deploySiteArgs.verboseSiteLogging)

	// Appending the configuration env var
	b, err := json.Marshal(rootConfNode)
	if err != nil {
		return nil, fmt.Errorf("Failed parsing configuraiton env var for orcs container - %s", err.Error())
	}
	confJson := string(b)

	log.Printf("orcs configuration json %s\n", confJson)

	rcRequest.Spec.Template.Spec.Containers[0].Env =
		append(
			rcRequest.Spec.Template.Spec.Containers[0].Env,
			v1.EnvVar{Name: "NAZ_MS_CONF", Value: confJson})

	var publicRoute string
	additionalPortsJSON := ""
	if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		for _, currPort := range svc.Spec.Ports {
			if currPort.Name == "service-http" {
				publicRoute = "http://" + extractLoadBalancerAddress(svc.Status.LoadBalancer) + ":" +
					strconv.Itoa(currPort.Port)
			} else {
				if additionalPortsJSON == "" {
					additionalPortsJSON = "{"
				} else {
					additionalPortsJSON += ","
				}
				// Adding env var telling the container which port has been mapped to the
				// requested additional port
				additionalPortsJSON += "\"" + currPort.Name + "\":\"" +
					strconv.Itoa(currPort.Port) + "\""
			}
		}
		if publicRoute == "" {
			return nil, errors.New("Failed assigning public port to service orcs")
		}

	} else if svc.Spec.Type == v1.ServiceTypeNodePort {
		for _, currPort := range svc.Spec.Ports {
			if currPort.Name == "service-http" {
				publicRoute = "http://" + ctx.ClusterIp + ":" + strconv.Itoa(currPort.NodePort)
			} else {
				// Adding env var telling the container which port has been mapped to the
				// requested additional port
				if additionalPortsJSON == "" {
					additionalPortsJSON = "{"
				} else {
					additionalPortsJSON += ","
				}
				// Adding env var telling the container which port has been mapped to the
				// requested additional port
				additionalPortsJSON += "\"" + currPort.Name + "\":\"" +
					strconv.Itoa(currPort.NodePort) + "\""
			}
		}
		if publicRoute == "" {
			return nil, errors.New("Failed assigning NodePort to service orcs")
		}
	}

	if publicRoute != "" {
		fmt.Printf("for service orcs - naz route - %s\n", publicRoute)
		rcRequest.Spec.Template.Spec.Containers[0].Env =
			append(
				rcRequest.Spec.Template.Spec.Containers[0].Env,
				v1.EnvVar{Name: "NAZ_PUBLIC_ROUTE", Value: publicRoute})
	}

	if ctx.DeploymentType == "local" {
		rcRequest.Spec.Template.Spec.Containers[0].Env =
			append(
				rcRequest.Spec.Template.Spec.Containers[0].Env,
				v1.EnvVar{Name: "LOCAL_CLUSTER_IP", Value: ctx.ClusterIp})

	}

	fmt.Println("Deploying the orcs replication controller")
	_, err = ctx.Client.DeployReplicationController("orcs", rcRequest, true)
	if err != nil {
		return nil, fmt.Errorf("Failed creating replication controller for orcs - %s", err.Error())
	}
	fmt.Println("Orcs replication controller has been deployed successfuly")
	time.Sleep(1 * time.Second)

	// Verifying all services have started
	err = waitForServiceToBeStarted(ctx, "site-api")
	if err != nil {
		return nil, err
	}
	err = waitForServiceToBeStarted(ctx, "hub-api")
	if err != nil {
		return nil, err
	}
	err = waitForServiceToBeStarted(ctx, "hub-web-api")
	if err != nil {
		return nil, err
	}
	err = waitForServiceToBeStarted(ctx, "protection-api")
	if err != nil {
		return nil, err
	}
	err = waitForServiceToBeStarted(ctx, "shpan-copy-store-api")
	if err != nil {
		return nil, err
	}

	return svc, nil

}

func createNamespace(ctx *cmd.DeployerContext, namespaceName string, cleanup bool) error {
	// In case cleanup is requested - dropping the entire site namespace
	if cleanup {
		fmt.Printf("Cleaning up namespace %s\n", namespaceName)
		err := ctx.Client.DeleteNamespaceAndWaitForTermination(namespaceName, 30, 10*time.Second)
		if err != nil {
			return err
		}
	}

	// Creating the k8s namespace we're going to use for deployment
	_, err := ctx.Client.CreateNamespace(
		&v1.Namespace{ObjectMeta: v1.ObjectMeta{Name: namespaceName}},
		true)
	if err != nil {
		return errors.New("Failed creating namespace " + err.Error())
	}
	fmt.Printf("Namespace %s created successfuly\n", ctx.Namespace)
	return nil

}

func deployMongoDsbCommandExecutor(ctx *cmd.DeployerContext) error {
	fmt.Printf("Creating namespace %s for deploying mongodsb\n", ctx.Namespace)
	err := createNamespace(ctx, ctx.Namespace, *deployMongoDsbArgs.cleanup)
	if err != nil {
		return errors.New("Failed creating namespace " + err.Error())
	}

	mongoDsbSvc, err := deployMongoDsbOrcs(ctx, true)
	if err != nil {
		return err
	}

	mongoDsbUrl, err := buildServiceRootUrl(mongoDsbSvc, ctx)
	if err != nil {
		return err
	}

	fmt.Printf("MongoDSB deployed at %s/dsb\n", mongoDsbUrl)

	return nil
}
func deployK8sPsbCommandExecutor(ctx *cmd.DeployerContext) error {
	fmt.Printf("Creating namespace %s for deploying k8spsb\n", ctx.Namespace)
	err := createNamespace(ctx, ctx.Namespace, *deployK8sPsbArgs.cleanup)
	if err != nil {
		return errors.New("Failed creating namespace " + err.Error())
	}

	k8sPsbSvc, err := deployK8sPsbOrcs(ctx, true)
	if err != nil {
		return err
	}

	k8sPsbUrl, err := buildServiceRootUrl(k8sPsbSvc, ctx)
	if err != nil {
		return err
	}

	fmt.Printf("k8spsb deployed at %s/k8spsb-api\n", k8sPsbUrl)

	return nil
}

func deploySiteCommandExecutor(ctx *cmd.DeployerContext) error {

	// Validating command arguments

	// In case no site name is supplied - site name becomes the kubernetes cluster ip
	if *deploySiteArgs.siteName == "" {
		if ctx.ClusterIp == "" {
			return errors.New("Site name is not defined, please use the site-name flag")
		}
		fmt.Println("No site name defined - defaulting to cluster ip - " + ctx.ClusterIp)
		*deploySiteArgs.siteName = ctx.ClusterIp
	}

	// In case cleanup is requested - dropping the entire site namespace
	if *deploySiteArgs.cleanup {
		fmt.Printf("Cleaning up namespace %s\n", ctx.Namespace)
		err := ctx.Client.DeleteNamespaceAndWaitForTermination(ctx.Namespace, 30, 10*time.Second)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Creating namespace %s for site %s\n", ctx.Namespace, *deploySiteArgs.siteName)
	err := createNamespace(ctx, ctx.Namespace, *deploySiteArgs.cleanup)
	if err != nil {
		return errors.New("Failed creating namespace " + err.Error())
	}

	// First we deploy an empty postgres used to store site data
	// todo: allow attaching to existing postgres (e.g. RDS pg)
	fmt.Println("Deploying postgres database onto the cluster to be used by the site")
	pgService, err := deployPostgres(ctx)
	if err != nil {
		return err
	}

	// Deploying the Orcs service
	orcsService, err := deployOrcsService(ctx, pgService)
	if err != nil {
		return err
	}

	// Building the root URL used by the orcs component
	rootUrl, err := buildServiceRootUrl(orcsService, ctx)
	if err != nil {
		return err
	}

	// Connecting site to the hub
	fmt.Printf("Configuring site %s to the local hub", *deploySiteArgs.siteName)
	err = postCommandOrcs(
		ctx,
		"hub-web",
		"add-site",
		resourceLocation{
			Urn: "site",
			Url: fmt.Sprintf("http://%s:%d/site-api",
				orcsService.Spec.ClusterIP,
				orcsService.Spec.Ports[0].Port),
		},
		http.StatusOK,
	)
	if err == nil {
		fmt.Printf("Site %s has been configured successfully with local hub\n", *deploySiteArgs.siteName)
	} else {
		return err
	}

	// Creating the lets-chat app template
	err = createAppTemplateOrcs(
		ctx,
		"lets-chat-template.json",
		"lets-chat.png",
	)
	if err != nil {
		return fmt.Errorf("Failed creating application template - %s", err.Error())
	}

	// Registering the default crb
	err = registerCrbOrcs(
		ctx,
		"shpan-copy-store",
		fmt.Sprintf(
			"http://%s:%d/shpan-copy-store-api",
			orcsService.Spec.ClusterIP,
			orcsService.Spec.Ports[0].Port))
	if err != nil {
		return err
	}

	// Deploying Kubernetes PSB
	k8sPsbSvc, err := deployK8sPsbOrcs(ctx, false)
	if err != nil {
		return err
	}
	// Deploying mongo DSB
	mongoDsbSvc, err := deployMongoDsbOrcs(ctx, false)
	if err != nil {
		return err
	}

	// Adding the dsb to the site
	err = registerDsbOrcs(ctx, "mongo-k8s-dsb", "http://"+mongoDsbSvc.Spec.ClusterIP+"/dsb")
	if err != nil {
		return err
	}

	err = registerPsbOrcs(ctx, "k8spsb", "http://"+k8sPsbSvc.Spec.ClusterIP+"/k8spsb-api")
	if err != nil {
		return err
	}

	// Adding the docker hub as docker artifact registry
	err = addDockerArtifactRegistryOrcs(ctx)

	/*
		err = deployK8sDsbOrcs(ctx)
		if (err != nil) {
			return err
		}
		err = deployK8sVolumeDsbOrcs(ctx)
		if (err != nil) {
			return err
		}
	*/

	fmt.Printf("Ocopea4k8s has been deployed!, wanna ride? at:\n%s/hub-web-api/html/nui/index.html\n", rootUrl)

	return nil
}

func buildServiceRootUrl(svc *v1.Service, ctx *cmd.DeployerContext) (string, error) {
	if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		return "http://" + extractLoadBalancerAddress(svc.Status.LoadBalancer), nil
	} else if svc.Spec.Type == v1.ServiceTypeNodePort {
		return "http://" + ctx.ClusterIp + ":" + strconv.Itoa(svc.Spec.Ports[0].NodePort), nil
	} else {
		return "", fmt.Errorf("Unsupported service type returned for orcs service %s\n", svc.Spec.Type)
	}

}

func createOrcsServicesConfiguration(
	pgService *v1.Service,
	siteName string,
	verboseLogging bool,
) configuration.StaticConfigurationNode {
	// building static configuration environment variable
	rootConfNode := configuration.StaticConfigurationNode{
		Children: make(map[string]configuration.StaticConfigurationNode),
	}

	dataSourceConfNode := configuration.StaticConfigurationNode{
		Children: make(map[string]configuration.StaticConfigurationNode),
	}
	blobStoreConfNode := configuration.StaticConfigurationNode{
		Children: make(map[string]configuration.StaticConfigurationNode),
	}
	serviceConfigConfNode := configuration.StaticConfigurationNode{
		Children: make(map[string]configuration.StaticConfigurationNode),
	}
	queueConfigNode := configuration.StaticConfigurationNode{
		Children: make(map[string]configuration.StaticConfigurationNode),
	}
	rootConfNode.Children["datasource"] = dataSourceConfNode
	rootConfNode.Children["blobstore"] = blobStoreConfNode
	rootConfNode.Children["serviceconfig"] = serviceConfigConfNode
	rootConfNode.Children["queue"] = queueConfigNode
	rootConfNode.Children["webserver"] = configuration.StaticConfigurationNode{
		Children: map[string]configuration.StaticConfigurationNode{
			"default": {
				Data: configuration.UndertowWebServerConfiguration{
					Port: "8080",
				},
			},
		},
	}

	// Register messaging system
	rootConfNode.Children["messaging"] = configuration.StaticConfigurationNode{
		Children: map[string]configuration.StaticConfigurationNode{
			"default-messaging": {
				Data: configuration.PersistentMessagingConfiguration{
					DatasourceName: "site-db",
					PersistMessage: "true",
				},
			},
		},
	}

	// Registering default scheduler
	rootConfNode.Children["scheduler"] = configuration.StaticConfigurationNode{
		Children: map[string]configuration.StaticConfigurationNode{
			"default": {
				Data: configuration.PersistentSchedulerConfiguration{
					Name:           "default",
					DatasourceName: "site-db",
					PersistTasks:   "true",
				},
			},
		},
	}

	// Register hub stuff
	buildHubServiceConfiguration(&rootConfNode, pgService, verboseLogging)
	buildSiteServiceConfiguration(&rootConfNode, pgService, siteName, verboseLogging)

	return rootConfNode

}

func getOrcsServiceUrl(ctx *cmd.DeployerContext) (string, error) {
	orcsService, err := ctx.Client.GetServiceInfo("orcs")
	if err != nil {
		return "", fmt.Errorf("Failed locating orcs service service - %s", err.Error())
	}

	return buildServiceRootUrl(orcsService, ctx)
}

func buildHubServiceConfiguration(
	rootConfig *configuration.StaticConfigurationNode,
	pgService *v1.Service,
	verboseLogging bool,
) {
	serviceConfigConfNode := rootConfig.Children["serviceconfig"]
	dataSourceConfNode := rootConfig.Children["datasource"]
	blobStoreConfNode := rootConfig.Children["blobstore"]

	hubDsConfNode := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server:         pgService.Spec.ClusterIP,
			Port:           strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName:   "postgres",
			DbUser:         "postgres",
			DbPassword:     "nazsecret",
			DbSchema:       "hub_repository",
			MaxConnections: "2",
		},
	}
	blobStoreDbConf := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server:         pgService.Spec.ClusterIP,
			Port:           strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName:   "postgres",
			DbUser:         "postgres",
			DbPassword:     "nazsecret",
			DbSchema:       "central_blobstore",
			MaxConnections: "2",
		},
	}

	dataSourceConfNode.Children["hub-db"] = hubDsConfNode
	blobStoreConfNode.Children["image-store"] = blobStoreDbConf

	var hubServiceParameters map[string]string = nil
	var hubWebServiceParameters map[string]string = nil
	if verboseLogging {
		hubServiceParameters = map[string]string{
			"print-all-json-requests": "true",
		}
		hubWebServiceParameters = map[string]string{
			"print-all-json-requests": "true",
		}
	}

	serviceConfigConfNode.Children["hub"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "hub",
			Route:      "http://localhost:8080/hub-api",
			DataSourceConfig: map[string]configuration.DataSourceConfig{
				"hub-db": {
					MaxConnections: 2,
				},
			},
			GlobalLoggingConfig: "DEBUG",
			Parameters:          hubServiceParameters,
		},
	}

	serviceConfigConfNode.Children["hub-web"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "hub-web",
			Route:      "http://localhost:8080/hub-web-api",
			BlobstoreConfig: map[string]configuration.DataSourceConfig{
				"image-store": {
					MaxConnections: 2,
				},
			},
			GlobalLoggingConfig: "DEBUG",
			Parameters:          hubWebServiceParameters,
		},
	}
}

func addQueue(rootConfig *configuration.StaticConfigurationNode, queueName string) {
	rootConfig.Children["queue"].Children[queueName] = configuration.StaticConfigurationNode{
		Data: configuration.PersistentQueueConfiguration{
			DestinationType:                     "QUEUE",
			QueueName:                           queueName,
			MemoryBufferMaxMessages:             "1000",
			SecondsToSleepBetweenMessageRetries: "2",
			MaxRetries:                          "2",
		},
	}
}
func buildSiteServiceConfiguration(
	rootConfig *configuration.StaticConfigurationNode,
	pgService *v1.Service,
	siteName string,
	verboseLogging bool) {
	serviceConfigConfNode := rootConfig.Children["serviceconfig"]
	dataSourceConfNode := rootConfig.Children["datasource"]
	blobStoreConfNode := rootConfig.Children["blobstore"]

	addQueue(rootConfig, "deployed-application-events")
	addQueue(rootConfig, "pending-deployed-application-events")
	addQueue(rootConfig, "application-copy-events")
	addQueue(rootConfig, "pending-application-copy-events")

	siteDsConfNode := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server:         pgService.Spec.ClusterIP,
			Port:           strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName:   "postgres",
			DbUser:         "postgres",
			DbPassword:     "nazsecret",
			DbSchema:       "site_repository",
			MaxConnections: "2",
		},
	}
	protectionDsConfNode := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server:         pgService.Spec.ClusterIP,
			Port:           strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName:   "postgres",
			DbUser:         "postgres",
			DbPassword:     "nazsecret",
			DbSchema:       "protection_repository",
			MaxConnections: "2",
		},
	}

	blobStoreDbConf := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server:         pgService.Spec.ClusterIP,
			Port:           strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName:   "postgres",
			DbUser:         "postgres",
			DbPassword:     "nazsecret",
			DbSchema:       "central_blobstore",
			MaxConnections: "2",
		},
	}

	dataSourceConfNode.Children["site-db"] = siteDsConfNode
	dataSourceConfNode.Children["protection-db"] = protectionDsConfNode
	blobStoreConfNode.Children["copy-store"] = blobStoreDbConf

	var siteServiceParameters map[string]string = map[string]string{
		"site-name": siteName,
		"location": "{" +
			"\"latitude\":32.1792126," +
			"\"longitude\":34.9005128," +
			"\"name\":\"Israel\"," +
			"\"properties\":{}}",
	}

	var protectionServiceParameters map[string]string = nil
	var shpanCopyStoreServiceParameters map[string]string = nil
	if verboseLogging {
		protectionServiceParameters = map[string]string{
			"print-all-json-requests": "true",
		}
		shpanCopyStoreServiceParameters = map[string]string{
			"print-all-json-requests": "true",
		}
		siteServiceParameters["print-all-json-requests"] = "true"
	}

	serviceConfigConfNode.Children["site"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "site",
			Route:      "http://localhost:8080/site-api",
			DataSourceConfig: map[string]configuration.DataSourceConfig{
				"site-db": {
					MaxConnections: 2,
				},
			},
			InputQueueConfig: map[string]configuration.InputQueueConfig{
				"deployed-application-events": {
					NumberOfConsumers: 5,
					LogInDebug:        true,
				},
				"pending-deployed-application-events": {
					NumberOfConsumers: 1,
					LogInDebug:        true,
				},
			},
			DestinationQueueConfig: map[string]configuration.DestinationQueueConfig{
				"deployed-application-events": {
					LogInDebug: true,
				},
				"pending-deployed-application-events": {
					LogInDebug: true,
				},
			},
			Parameters:          siteServiceParameters,
			GlobalLoggingConfig: "DEBUG",
		},
	}

	// Adding protection service
	serviceConfigConfNode.Children["protection"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "protection",
			Route:      "http://localhost:8080/protection-api",
			DataSourceConfig: map[string]configuration.DataSourceConfig{
				"protection-db": {
					MaxConnections: 2,
				},
			},
			InputQueueConfig: map[string]configuration.InputQueueConfig{
				"application-copy-events": {
					NumberOfConsumers: 5,
					LogInDebug:        true,
				},
				"pending-application-copy-events": {
					NumberOfConsumers: 1,
					LogInDebug:        true,
				},
			},
			DestinationQueueConfig: map[string]configuration.DestinationQueueConfig{
				"application-copy-events": {
					LogInDebug: true,
				},
				"pending-application-copy-events": {
					LogInDebug: true,
				},
			},
			Parameters:          protectionServiceParameters,
			GlobalLoggingConfig: "DEBUG",
		},
	}

	// Adding shpan-copy-store
	serviceConfigConfNode.Children["shpan-copy-store"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "shpan-copy-store",
			Route:      "http://localhost:8080/shpan-copy-store-api",
			BlobstoreConfig: map[string]configuration.DataSourceConfig{
				"copy-store": {
					MaxConnections: 2,
				},
			},
			GlobalLoggingConfig: "DEBUG",
			Parameters:          shpanCopyStoreServiceParameters,
		},
	}
}

func deployK8sPsbOrcs(ctx *cmd.DeployerContext, exposePublic bool) (*v1.Service, error) {
	fmt.Println("Deploying k8s-psb")
	// Verifying site is registered
	svc, err := deployService(
		ctx.Client,
		"k8spsb",
		ctx.DeploymentType,
		exposePublic,
		true,
		[]v1.EnvVar{
			{Name: "K8S_USERNAME", Value: ctx.Client.UserName},
			{Name: "K8S_PASSWORD", Value: ctx.Client.Password},
			{Name: "OCOPEA_NAMESPACE", Value: ctx.Client.Namespace},
		},
		80,
		8080,
		[]v1.ServicePort{},
		"ocopea/go-k8s-psb",
		"psb",
		ctx.ClusterIp)

	if err != nil {
		return nil, err
	}

	serviceUrl := "http://" + svc.Spec.ClusterIP + "/k8spsb-api"
	fmt.Printf("k8spsb deployed on ip %s\n", serviceUrl)

	return svc, err
}

func deployMongoDsbOrcs(ctx *cmd.DeployerContext, exposePublic bool) (*v1.Service, error) {

	fmt.Println("Deploying mongo-dsb")
	// Verifying site is registered on hub
	svc, err := deployService(
		ctx.Client,
		"mongo-k8s-dsb",
		ctx.DeploymentType,
		exposePublic,
		true,
		[]v1.EnvVar{
			{Name: "K8S_USERNAME", Value: ctx.Client.UserName},
			{Name: "K8S_PASSWORD", Value: ctx.Client.Password},
			{Name: "OCOPEA_NAMESPACE", Value: ctx.Client.Namespace},
			{Name: "PORT", Value: "8080"},
			{Name: "HOST", Value: "0.0.0.0"},
		},
		80,
		8080,
		[]v1.ServicePort{},
		"ocopea/mongo-k8s-dsb",
		"dsb",
		ctx.ClusterIp,
	)

	if err != nil {
		return nil, err
	}
	fmt.Println("mongo-dsb has been deployed successfully")

	return svc, nil
}

func deployK8sDsbOrcs(ctx *cmd.DeployerContext) error {
	// Verifying site is registered on hub
	svc, err := deployService(
		ctx.Client,
		"k8sdsb",
		ctx.DeploymentType,
		false,
		true,
		[]v1.EnvVar{
			{Name: "K8S_USERNAME", Value: ctx.Client.UserName},
			{Name: "K8S_PASSWORD", Value: ctx.Client.Password},
			{Name: "OCOPEA_NAMESPACE", Value: ctx.Client.Namespace},
			{Name: "PORT", Value: "8080"},
			{Name: "HOST", Value: "0.0.0.0"},
		},
		80,
		8080,
		[]v1.ServicePort{},
		"ocopea/k8s-dsb",
		"dsb",
		ctx.ClusterIp,
	)

	if err != nil {
		return err
	}

	// Adding the dsb to the site
	err = registerDsbOrcs(ctx, "k8s-dsb", "http://"+svc.Spec.ClusterIP+"/dsb")
	if err != nil {
		return err
	}

	return err

}

func deployK8sVolumeDsbOrcs(ctx *cmd.DeployerContext) error {
	// Verifying site is registered on hub
	svc, err := deployService(
		ctx.Client,
		"k8svolumedsb",
		ctx.DeploymentType,
		false,
		true,
		[]v1.EnvVar{
			{Name: "K8S_USERNAME", Value: ctx.Client.UserName},
			{Name: "K8S_PASSWORD", Value: ctx.Client.Password},
			{Name: "OCOPEA_NAMESPACE", Value: ctx.Client.Namespace},
			{Name: "PORT", Value: "8080"},
			{Name: "HOST", Value: "0.0.0.0"},
		},
		80,
		8080,
		[]v1.ServicePort{},
		"ocopea/k8s-volume-dsb",
		"dsb",
		ctx.ClusterIp,
	)

	if err != nil {
		return err
	}

	// Adding the dsb to the site
	err = registerDsbOrcs(ctx, "k8s-volume-dsb", "http://"+svc.Spec.ClusterIP+"/dsb")
	if err != nil {
		return err
	}

	return err

}

func registerDsbOrcs(ctx *cmd.DeployerContext, dsbUrn string, dsbUrl string) error {
	fmt.Printf("Registering DSB %s on site using url %s\n", dsbUrn, dsbUrl)

	err, siteId := getSiteIdOrcs(ctx, "site")
	if err != nil {
		return err
	}
	err = postCommandOrcs(
		ctx,
		"hub-web",
		"add-dsb",
		UICommandAddDsb{
			DsbUrn: dsbUrn,
			DsbUrl: dsbUrl,
			SiteId: siteId,
		},
		http.StatusOK)

	if err == nil {
		fmt.Printf("DSB %s has been registered on site successfully\n", dsbUrn)
	}
	return err

}

func registerPsbOrcs(ctx *cmd.DeployerContext, psbUrn string, psbUrl string) error {
	fmt.Printf("Registering PSB %s on site using url %s\n", psbUrn, psbUrl)

	err, siteId := getSiteIdOrcs(ctx, "site")
	if err != nil {
		return err
	}
	// Registering PSB on site
	fmt.Println("Registering cf-psb on site")
	err = postCommandOrcs(
		ctx,
		"hub-web",
		"add-psb",
		UICommandAddPsb{
			PsbUrn: psbUrn,
			PsbUrl: psbUrl,
			SiteId: siteId,
		},
		http.StatusNoContent)

	if err != nil {
		return err
	}
	fmt.Printf("%s registered successfully on site\n", psbUrn)

	return err

}

func registerCrbOrcs(ctx *cmd.DeployerContext, crbUrn string, crbUrl string) error {
	fmt.Printf("Registering crb %s on site using url %s\n", crbUrn, crbUrl)
	err, siteId := getSiteIdOrcs(ctx, "site")
	if err != nil {
		return err
	}
	err = postCommandOrcs(
		ctx,
		"hub-web",
		"add-crb",
		UICommandAddCrb{
			CrbUrn: crbUrn,
			CrbUrl: crbUrl,
			SiteId: siteId,
		},
		http.StatusNoContent)

	if err == nil {
		fmt.Printf("CRB %s has been registered on site\n", crbUrn)
	}
	return err
}

func postCommandOrcs(
	ctx *cmd.DeployerContext,
	svcUrn string,
	commandName string,
	jsonBody interface{},
	expectedStatusCode int) error {
	orcsServiceUrl, err := getOrcsServiceUrl(ctx)
	if err != nil {
		return fmt.Errorf("Failed getting orcs service url - %s", err.Error())
	}

	b := new(bytes.Buffer)
	enc := json.NewEncoder(b)
	err = enc.Encode(jsonBody)
	if err != nil {
		return fmt.Errorf("Failed encoding resource info for command %s - %s", commandName, err.Error())
	}

	// Posting the command
	urlToPost := orcsServiceUrl + "/" + svcUrn + "-api/commands/" + commandName
	log.Printf("Posting to %s\n", urlToPost)
	req, err := http.NewRequest(
		"POST",
		urlToPost,
		b)
	if err != nil {
		return fmt.Errorf("Failed posting command %s to %s - %s", commandName, svcUrn, err.Error())
	}

	resp, err := http.DefaultClient.Do(prepareOcopeaRequest(req))
	if err != nil {
		return fmt.Errorf("Failed executing command %s on %s - %s", commandName, svcUrn, err.Error())
	}

	jb, err := json.MarshalIndent(jsonBody, "", "    ")
	if err == nil {
		log.Printf("Post to %s\n%s\n returned %s\n", urlToPost, string(jb), resp.Status)
	}
	if resp.StatusCode != expectedStatusCode {
		return fmt.Errorf("Failed executing command %s on %s - %s", commandName, svcUrn, resp.Status)
	}

	return nil
}

func addDockerArtifactRegistryOrcs(ctx *cmd.DeployerContext) error {

	fmt.Println("configuring the docker hub as the sites default artifact registry")

	err, siteId := getSiteIdOrcs(ctx, "site")
	if err != nil {
		return err
	}

	err = postCommandOrcs(
		ctx,
		"hub-web",
		"add-docker-artifact-registry",
		UICommandAddDockerArtifactRegistry{
			Name:   "shpanRegistry",
			Url:    "https://registry.hub.docker.com",
			SiteId: siteId,
		},
		http.StatusNoContent)

	if err == nil {
		fmt.Println("docker hub has been configured successfully")
	}
	return err
}

func defineDeploySiteCommand() *cmd.DeployerCommand {
	cmd := &cmd.DeployerCommand{
		Name:     "deploy-site",
		Executor: deploySiteCommandExecutor,
	}

	cmd.FlagSet = flag.NewFlagSet(cmd.Name, flag.ExitOnError)

	deploySiteArgs = &deploySiteArgsBag{
		siteName:           cmd.FlagSet.String("site-name", "", "Site Name"),
		cleanup:            cmd.FlagSet.Bool("cleanup", false, "Cleanup Namespace before deploying"),
		verboseSiteLogging: cmd.FlagSet.Bool("verbose-site-logging", false, "Make the site logging verbose"),
	}
	return cmd
}

func defineDeployMongoDsbCommand() *cmd.DeployerCommand {
	cmd := &cmd.DeployerCommand{
		Name:     "deploy-mongodsb",
		Executor: deployMongoDsbCommandExecutor,
	}

	cmd.FlagSet = flag.NewFlagSet(cmd.Name, flag.ExitOnError)

	deployMongoDsbArgs = &deployServiceArgsBag{
		cleanup: cmd.FlagSet.Bool("cleanup", false, "Cleanup Namespace before deploying"),
	}
	return cmd
}
func defineDeployK8sPsbCommand() *cmd.DeployerCommand {
	cmd := &cmd.DeployerCommand{
		Name:     "deploy-k8spsb",
		Executor: deployK8sPsbCommandExecutor,
	}

	cmd.FlagSet = flag.NewFlagSet(cmd.Name, flag.ExitOnError)

	deployK8sPsbArgs = &deployServiceArgsBag{
		cleanup: cmd.FlagSet.Bool("cleanup", false, "Cleanup Namespace before deploying"),
	}
	return cmd
}

func main() {

	f, err := os.OpenFile("deployer.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)

	// define all the supported commands
	deployerCommands := []*cmd.DeployerCommand{
		defineDeploySiteCommand(),
		defineDeployK8sPsbCommand(),
		defineDeployMongoDsbCommand(),
	}

	// Executing the command selected by the user or show prompt
	err = cmd.Exec(deployerCommands)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}

func createReplicationControllerStruct(
	serviceName string,
	deploymentType string,
	imageName string,
	nazKind string) (*v1.ReplicationController, error) {
	var replicas int = 1
	// Building rc spec

	spec := v1.ReplicationControllerSpec{}
	spec.Replicas = &replicas
	spec.Selector = make(map[string]string)
	spec.Selector["app"] = serviceName
	spec.Template = &v1.PodTemplateSpec{}
	spec.Template.ObjectMeta = v1.ObjectMeta{}
	spec.Template.ObjectMeta.Labels = make(map[string]string)
	spec.Template.ObjectMeta.Labels["app"] = serviceName

	if nazKind != "" {
		spec.Template.ObjectMeta.Labels["nazKind"] = nazKind
	}

	containerSpec := v1.Container{}
	containerSpec.Name = serviceName

	containerSpec.Image = imageName
	containerSpec.ImagePullPolicy = v1.PullIfNotPresent
	containerSpec.Ports = []v1.ContainerPort{{ContainerPort: 8080}}
	containerSpec.Env = []v1.EnvVar{{Name: "OCOPEA_DEPLOYMENT_TYPE", Value: deploymentType}}
	containers := []v1.Container{containerSpec}

	spec.Template.Spec = v1.PodSpec{}
	spec.Template.Spec.Containers = containers

	// Create a replicationController object for running the app
	rc := &v1.ReplicationController{}
	rc.Name = serviceName
	rc.Labels = make(map[string]string)
	if nazKind != "" {
		rc.Labels["nazKind"] = nazKind
	}
	rc.Spec = spec
	return rc, nil

}
func createK8SServiceStruct(
	serviceName string,
	deploymentType string,
	exposePublic bool,
	mainPort int,
	mainContainerPort int,
	additionalPorts []v1.ServicePort) (*v1.Service, error) {
	svc := &v1.Service{}
	svc.Name = serviceName

	var svcType v1.ServiceType
	if exposePublic {
		switch deploymentType {
		case "local":
			svcType = v1.ServiceTypeNodePort
			break
		case "aws":
			fallthrough
		case "gce":
			fallthrough
		default:
			svcType = v1.ServiceTypeLoadBalancer
			break

		}
	} else {
		svcType = v1.ServiceTypeClusterIP
	}
	svc.Spec.Type = svcType
	svc.Spec.Ports = []v1.ServicePort{{
		Port:       mainPort,
		TargetPort: types.NewIntOrStringFromInt(mainContainerPort),
		Protocol:   v1.ProtocolTCP,
		Name:       "service-http"}}

	// Appending additional ports where applicable
	for _, currPort := range additionalPorts {
		svc.Spec.Ports =
			append(
				svc.Spec.Ports,
				currPort)
	}

	svc.Spec.Selector = map[string]string{"app": serviceName}

	return svc, nil

}

func deployService(
	client *k8sClient.Client,
	serviceName string,
	deploymentType string,
	exposePublic bool,
	force bool,
	additionalVars []v1.EnvVar,
	mainPort int,
	mainContainerPort int,
	additionalPorts []v1.ServicePort,
	imageName string,
	nazKind string,
	clusterIp string) (*v1.Service, error) {

	svc, err := createK8SServiceStruct(
		serviceName,
		deploymentType,
		exposePublic,
		mainPort,
		mainContainerPort,
		additionalPorts)

	if err != nil {
		return nil, err
	}

	svc, err = deployK8SService(client, svc, force)
	if err != nil {
		return nil, err
	}

	rcRequest, err := createReplicationControllerStruct(serviceName, deploymentType, imageName, nazKind)
	if err != nil {
		return nil, fmt.Errorf(
			"Failed reading data from replication controller file for %s - %s", serviceName, err.Error())
	}

	for _, currVar := range additionalVars {
		rcRequest.Spec.Template.Spec.Containers[0].Env =
			append(
				rcRequest.Spec.Template.Spec.Containers[0].Env,
				currVar)

	}

	// Adding all "additionalPorts" to container ports
	for _, currPort := range additionalPorts {
		rcRequest.Spec.Template.Spec.Containers[0].Ports = append(
			rcRequest.Spec.Template.Spec.Containers[0].Ports,
			v1.ContainerPort{ContainerPort: currPort.TargetPort.IntVal})
	}

	var publicRoute string
	additionalPortsJSON := ""
	if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		for _, currPort := range svc.Spec.Ports {
			if currPort.Name == "service-http" {
				publicRoute = "http://" + extractLoadBalancerAddress(svc.Status.LoadBalancer) + ":" +
					strconv.Itoa(currPort.Port)
			} else {
				if additionalPortsJSON == "" {
					additionalPortsJSON = "{"
				} else {
					additionalPortsJSON += ","
				}
				// Adding env var telling the container which port has been mapped to the
				// requested additional port
				additionalPortsJSON += "\"" + currPort.Name + "\":\"" +
					strconv.Itoa(currPort.Port) + "\""
			}
		}
		if publicRoute == "" {
			return nil, fmt.Errorf("Failed assigning public port to service %s", serviceName)
		}

	} else if svc.Spec.Type == v1.ServiceTypeNodePort {
		for _, currPort := range svc.Spec.Ports {
			if currPort.Name == "service-http" {
				publicRoute = "http://" + clusterIp + ":" + strconv.Itoa(currPort.NodePort)
			} else {
				// Adding env var telling the container which port has been mapped to the
				// requested additional port
				if additionalPortsJSON == "" {
					additionalPortsJSON = "{"
				} else {
					additionalPortsJSON += ","
				}
				// Adding env var telling the container which port has been mapped to the requested
				// additional port
				additionalPortsJSON += "\"" + currPort.Name + "\":\"" +
					strconv.Itoa(currPort.NodePort) + "\""
			}
		}
		if publicRoute == "" {
			return nil, fmt.Errorf("Failed assigning NodePort to service %s", serviceName)
		}
	}

	if publicRoute != "" {
		log.Printf("for service %s - naz route - %s\n", serviceName, publicRoute)
		rcRequest.Spec.Template.Spec.Containers[0].Env =
			append(
				rcRequest.Spec.Template.Spec.Containers[0].Env,
				v1.EnvVar{Name: "NAZ_PUBLIC_ROUTE", Value: publicRoute})
	}

	if additionalPortsJSON != "" {
		additionalPortsJSON += "}"
		log.Printf("Additional ports %s\n", additionalPortsJSON)
		rcRequest.Spec.Template.Spec.Containers[0].Env = append(
			rcRequest.Spec.Template.Spec.Containers[0].Env,
			v1.EnvVar{Name: "NAZ_PORTS", Value: additionalPortsJSON})

	}

	if deploymentType == "local" {
		rcRequest.Spec.Template.Spec.Containers[0].Env =
			append(
				rcRequest.Spec.Template.Spec.Containers[0].Env,
				v1.EnvVar{Name: "LOCAL_CLUSTER_IP", Value: clusterIp})

	}

	_, err = client.DeployReplicationController(serviceName, rcRequest, force)
	if err != nil {
		return nil, fmt.Errorf("Failed creating replication controller for %s - %s", serviceName, err.Error())
	}

	return svc, nil

}

func deployK8SService(client *k8sClient.Client, svc *v1.Service, force bool) (*v1.Service, error) {
	svc, err := client.CreateService(svc, force)
	if err != nil {
		return nil, err
	}

	svc, err = client.WaitForServiceToStart(svc.Name, 100, 3*time.Second)
	if err != nil {
		return svc, err
	}
	log.Printf("Service %s deployed successfully\n", svc.Name)
	return svc, nil

}

func createAppTemplateOrcs(ctx *cmd.DeployerContext, templatePath string, iconPath string) error {
	fmt.Printf("creating the application template using %s, icon:%s\n", templatePath, iconPath)
	orcsServiceUrl, err := getOrcsServiceUrl(ctx)
	if err != nil {
		return err
	}
	commandUrl := orcsServiceUrl + "/hub-web-api/commands/create-app-template"
	log.Println(commandUrl)

	templateFile, err := os.Open(templatePath)
	if err != nil {
		return fmt.Errorf("Failed reading app template path %s - %s", templatePath, err.Error())
	}
	req, err := http.NewRequest(
		"POST",
		commandUrl,
		templateFile)
	if err != nil {
		return fmt.Errorf("Failed creating app template - %s", err.Error())
	}

	resp, err := http.DefaultClient.Do(prepareOcopeaRequest(req))

	if err != nil {
		return fmt.Errorf("Failed posting new app template - %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusCreated &&
		resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("Failed creating app template - %s", resp.Status)
	}

	defer resp.Body.Close()
	btt, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed reading create template response " + err.Error())
	}

	if resp.StatusCode != http.StatusConflict {
		appTemplateId := strings.Replace(string(btt), "\"", "", 2)

		f, err := os.Open(iconPath)
		if err != nil {
			log.Println("Failed reading icon path - " + err.Error())
		}
		postIconUrl := orcsServiceUrl + "/hub-web-api/images/app-template/" + appTemplateId
		log.Println("Posting icon to " + postIconUrl)
		reqIcon, err := http.NewRequest(
			"POST",
			postIconUrl,
			f)

		if err != nil {
			log.Println("Failed reading create post for icon - " + err.Error())
		}
		_, err = http.DefaultClient.Do(prepareOcopeaRequestWithContentType(reqIcon, "application/octet-stream"))
		if err != nil {
			log.Println("Failed posting app template icon - " + err.Error())
		}
	}
	fmt.Printf("successfully created application template using %s, icon:%s\n", templatePath, iconPath)
	return nil
}

func getSiteIdOrcs(ctx *cmd.DeployerContext, siteUrn string) (error, string) {
	orcsUrl, err := getOrcsServiceUrl(ctx)
	if err != nil {
		return err, ""
	}
	req, err := http.NewRequest(
		"GET",
		orcsUrl+"/hub-web-api/site",
		nil)
	if err != nil {
		return fmt.Errorf("Failed createing app template req - %s", err.Error()), ""
	}

	resp, err := http.DefaultClient.Do(prepareOcopeaRequest(req))

	if err != nil {
		return fmt.Errorf("Failed posting new app template - %s", err.Error()), ""
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Failed creating app template - %s", resp.Status), ""
	}

	var sitesArray []UISite
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&sitesArray)
	if err != nil {
		return fmt.Errorf("Failed decoding sits array - %s", err.Error()), ""
	}

	//todo: only do on debug
	str, err := json.Marshal(sitesArray)
	if err != nil {
		log.Println(err)
	}
	log.Printf(string(str))

	for _, currSite := range sitesArray {
		if currSite.Urn == siteUrn {
			return nil, currSite.Id
		}
	}

	return errors.New("Could not find site with urn " + siteUrn), ""
}

func waitForServiceToBeStarted(ctx *cmd.DeployerContext, serviceEndpoint string) error {
	fmt.Printf("Waiting for service %s to start\n", serviceEndpoint)
	var err error = nil
	for i := 1; i < 100; i++ {
		err = verifyOrcsServiceHasStarted(ctx, serviceEndpoint)
		if err == nil {
			fmt.Printf("Service %s started successfully\n", serviceEndpoint)
			return nil
		} else {
			log.Printf("service %s is not ready yet - %s\n", serviceEndpoint, err.Error())
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("service endpoint %s failed starting in a timely fasion - %s", serviceEndpoint, err.Error())
}

func verifyOrcsServiceHasStarted(ctx *cmd.DeployerContext, serviceEndpoint string) error {
	orcsServiceUrl, err := getOrcsServiceUrl(ctx)
	if err != nil {
		return fmt.Errorf("Failed getting orcs service url - %s", err.Error())
	}

	stateUrl := orcsServiceUrl + "/" + serviceEndpoint + "/state"
	log.Printf("Verifying service %s is running by getting it's status from %s\n", serviceEndpoint, stateUrl)

	req, err := http.NewRequest("GET", stateUrl, nil)
	if err != nil {
		return fmt.Errorf("failed creating get request on %s - %s", stateUrl, err.Error())
	}

	resp, err := http.DefaultClient.Do(prepareOcopeaRequest(req))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	all, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Println(string(all))

	var dat map[string]interface{}
	json.Unmarshal([]byte(all), &dat)
	stateObj := dat["state"]
	if stateObj == nil {
		return fmt.Errorf(
			"service state endpint for service %s, didn't contain a \"state\" field",
			serviceEndpoint)
	}
	state := stateObj.(string)
	fmt.Println(state)
	if strings.Compare(state, "RUNNING") != 0 {
		return fmt.Errorf("site is not running but %s", state)
	}
	return nil
}

func extractLoadBalancerAddress(loadBalancerStatus v1.LoadBalancerStatus) string {
	if len(loadBalancerStatus.Ingress[0].IP) > 0 {
		return loadBalancerStatus.Ingress[0].IP
	}
	if len(loadBalancerStatus.Ingress[0].Hostname) > 0 {
		return loadBalancerStatus.Ingress[0].Hostname
	}
	return ""
}

func prepareOcopeaRequestWithContentType(req *http.Request, contentType string) *http.Request {
	req.Header.Add("Content-Type", contentType)
	req.SetBasicAuth(
		OCOPEA_ADMIN_USERNAME,
		OCOPEA_ADMIN_PASSWORD,
	)
	return req

}
func prepareOcopeaRequest(req *http.Request) *http.Request {
	return prepareOcopeaRequestWithContentType(req, "application/json")
}
