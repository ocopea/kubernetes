package main

import (
	"flag"
	"fmt"
	k8sClient "ocopea/kubernetes/client"
	"os"
	"time"
	"log"
	"encoding/json"
	"ocopea/kubernetes/client/v1"
	"io/ioutil"
	"strings"
	"ocopea/kubernetes/client/types"
	"strconv"
	"errors"
	"ocopea/kubernetes/deployer/configuration"
	"net/http"
	"bytes"
	"ocopea/kubernetes/deployer/cmd"
)
/**
This utility deploys ocopea site instance to a k8s cluster
It assumes all docker images are built and published to a repository accessible by the k8s cluster
 */

type deployServiceInfo struct {
	ServiceName                string
	ImageName                  string
	EnvVars                    map[string]string
	ServiceRegistrationDetails []serviceRegistrationDetail
}

type deploySiteArgsBag struct {
	siteName *string
	cleanup  *bool
}

var deploySiteArgs *deploySiteArgsBag


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

func deployPostgres(ctx *cmd.DeployerContext) (error) {
	// Deploy pg service
	pgSvc, err := deployService(
		ctx.Client,
		"nazdb",
		ctx.DeploymentType,
		false,
		true,
		[]v1.EnvVar{{
			Name: "POSTGRES_PASSWORD",
			Value: "nazsecret",
		}},
		5432,
		5432,
		[]v1.ServicePort{},
		"postgres",
		"infra",
		ctx.ClusterIp)

	if err != nil {
		return err
	}

	fmt.Println("postgres is here " + pgSvc.Spec.ClusterIP)
	return err

}

func deploySite(ctx *cmd.DeployerContext) (error) {

	// In case no site name is supplied - site name becomes the kubernetes cluster ip
	if (*deploySiteArgs.siteName == "") {
		log.Println("No site name defined - defaulting to cluster ip - " + ctx.ClusterIp)
		*deploySiteArgs.siteName = ctx.ClusterIp
	}

	// In case cleanup is requested - dropping the entire site namespace
	if (*deploySiteArgs.cleanup) {
		fmt.Printf("Cleaning up namespace %s\n", ctx.Namespace)
		err := ctx.Client.DeleteNamespaceAndWaitForTermination(ctx.Namespace, 30, 10 * time.Second)
		if (err != nil) {
			return err
		}
	}

	// Creating the k8s namespace we're going to use for deployment
	fmt.Printf("Creating namespace %s for site %s\n", ctx.Namespace, *deploySiteArgs.siteName)
	_, err := ctx.Client.CreateNamespace(
		&v1.Namespace{ObjectMeta: v1.ObjectMeta{ Name: ctx.Namespace}},
		false)
	if (err != nil) {
		return errors.New("Failed creating namespace " + err.Error())
	}
	fmt.Printf("Namespace %s created successfuly\n", ctx.Namespace)

	// First we deploy an empty postgres used to store site data
	// todo: allow attaching to existing postgres (e.g. RDS pg)

	fmt.Println("Deploying postgres database onto the cluster to be used by the site")
	err = deployPostgres(ctx)
	if err != nil {
		return err;
	}

	// verifying postgres is started
	pgService, err := ctx.Client.WaitForServiceToStart("nazdb", 100, 3 * time.Second)
	if (err != nil) {
		return err;
	}
	fmt.Printf("postgres deployed at %s\n", pgService.Spec.ClusterIP)

	// Creating the service descriptor
	svc, err := createK8SServiceStruct(
		"orcs",
		ctx.DeploymentType,
		true,
		80,
		8080,
		[]v1.ServicePort{})

	if err != nil {
		return err
	}

	// Deploying the service onto k8s and waiting for it to start
	fmt.Println("deploying the orcs service");
	svc, err = deployK8SService(ctx.Client, svc, true)
	if (err != nil) {
		return err
	}

	var serviceUrl string;
	if (svc.Spec.Type == v1.ServiceTypeLoadBalancer) {
		serviceUrl =
			"http://" + extractLoadBalancerAddress(svc.Status.LoadBalancer) + ":80"
	} else if (svc.Spec.Type == v1.ServiceTypeNodePort) {
		serviceUrl = "http://" + ctx.ClusterIp + ":" +
			strconv.Itoa(svc.Spec.Ports[0].NodePort)
	} else {
		return errors.New("Failed locating public Url when deploying orcs")
	}

	fmt.Println("orcs service deployed successfuly")
	log.Printf("orcs service deployed to %s/hub-web-api/html/ui/index.html\n", serviceUrl)

	rcRequest, err := createReplicationControllerStruct(
		"orcs",
		ctx.DeploymentType,
		"ocopea/orcs-k8s-runner",
		"orcs");

	if (err != nil) {
		return fmt.Errorf("Failed creating replication controller for orcs - %s", err.Error());
	}

	//todo:amit: I don't think we need this in orcs container
	rcRequest.Spec.Template.Spec.Containers[0].Env =
		append(
			rcRequest.Spec.Template.Spec.Containers[0].Env,
			v1.EnvVar{Name:"K8S_USERNAME", Value: ctx.Client.UserName},
			v1.EnvVar{Name:"K8S_PASSWORD", Value: ctx.Client.Password},
			v1.EnvVar{Name:"NAZ_NAMESPACE", Value: ctx.Client.Namespace})

	rootConfNode := createOrcsServicesConfiguration(pgService, *deploySiteArgs.siteName)

	// Appending the configuration env var
	b, err := json.Marshal(rootConfNode)
	if (err != nil) {
		return fmt.Errorf("Failed parsing configuraiton env var for orcs container - %s", err.Error());
	}
	confJson := string(b)

	log.Printf("orcs configuration json %s\n", confJson)

	rcRequest.Spec.Template.Spec.Containers[0].Env =
		append(
			rcRequest.Spec.Template.Spec.Containers[0].Env,
			v1.EnvVar{Name:"NAZ_MS_CONF", Value: confJson})


	// todo:amit:check what is this NAZ_PUBLIC_ROUTE shit
	var publicRoute string
	additionalPortsJSON := ""
	if (svc.Spec.Type == v1.ServiceTypeLoadBalancer) {
		for _, currPort := range svc.Spec.Ports {
			if (currPort.Name == "service-http") {
				publicRoute = "http://" + extractLoadBalancerAddress(svc.Status.LoadBalancer) + ":" +
					strconv.Itoa(currPort.Port)
			} else {
				if (additionalPortsJSON == "") {
					additionalPortsJSON = "{"
				} else {
					additionalPortsJSON += ","
				}
				// Adding env var telling the container which port has been mapped to the requested additional port
				additionalPortsJSON += "\"" + currPort.Name + "\":\"" + strconv.Itoa(currPort.Port) + "\"";
			}
		}
		if publicRoute == "" {
			return errors.New("Failed assigning public port to service orcs");
		}

	} else if (svc.Spec.Type == v1.ServiceTypeNodePort) {
		for _, currPort := range svc.Spec.Ports {
			if (currPort.Name == "service-http") {
				publicRoute = "http://" + ctx.ClusterIp + ":" + strconv.Itoa(currPort.NodePort)
			} else {
				// Adding env var telling the container which port has been mapped to the requested additional port
				if (additionalPortsJSON == "") {
					additionalPortsJSON = "{"
				} else {
					additionalPortsJSON += ","
				}
				// Adding env var telling the container which port has been mapped to the requested additional port
				additionalPortsJSON += "\"" + currPort.Name + "\":\"" + strconv.Itoa(currPort.NodePort) + "\"";
			}
		}
		if publicRoute == "" {
			return errors.New("Failed assigning NodePort to service orcs");
		}
	}

	if (publicRoute != "") {
		fmt.Printf("for service orcs - naz route - %s", publicRoute)
		rcRequest.Spec.Template.Spec.Containers[0].Env =
			append(
				rcRequest.Spec.Template.Spec.Containers[0].Env,
				v1.EnvVar{Name:"NAZ_PUBLIC_ROUTE", Value: publicRoute})
	}

	if (ctx.DeploymentType == "local") {
		rcRequest.Spec.Template.Spec.Containers[0].Env =
			append(
				rcRequest.Spec.Template.Spec.Containers[0].Env,
				v1.EnvVar{Name:"LOCAL_CLUSTER_IP", Value: ctx.ClusterIp})

	}

	fmt.Println("Deploying the orcs replication controller")
	_, err = deployK8SReplicationController(ctx.Client, "orcs", rcRequest, true)
	if (err != nil) {
		return fmt.Errorf("Failed creating replication controller for orcs - %s", err.Error());
	}
	fmt.Println("Orcs replication controller has been deployed successfuly")
	time.Sleep(1 * time.Second)

	var rootUrl string;
	if (svc.Spec.Type == v1.ServiceTypeLoadBalancer) {
		//todo:2: read correct port from service
		rootUrl = "http://" + extractLoadBalancerAddress(svc.Status.LoadBalancer)
	} else if (svc.Spec.Type == v1.ServiceTypeNodePort) {
		rootUrl = "http://" + ctx.ClusterIp + ":" + strconv.Itoa(svc.Spec.Ports[0].NodePort)
	} else {
		return fmt.Errorf("Unsupported service type returned for root service %s\n", svc.Spec.Type)
	}

	// Verifying all services have started
	waitForServiceToBeStarted(ctx, "site-api")
	waitForServiceToBeStarted(ctx, "hub-api")
	waitForServiceToBeStarted(ctx, "hub-web-api")
	waitForServiceToBeStarted(ctx, "protection-api")
	waitForServiceToBeStarted(ctx, "shpan-copy-store-api")

	// Connecting site to the hub
	fmt.Printf("Configuring site %s to the local hub", *deploySiteArgs.siteName)
	err = postCommandOrcs(
		ctx,
		"hub-web",
		"add-site",
		resourceLocation{
			Urn: "site",
			Url: fmt.Sprintf("http://%s:%d/site-api", svc.Spec.ClusterIP, svc.Spec.Ports[0].Port),
		},
		http.StatusOK,
	);

	if err == nil {
		fmt.Printf("Site %s has been configured successfully with local hub", *deploySiteArgs.siteName)
	} else {
		return err
	}

	// Creating app template
	err = createAppTemplateOrcs(
		ctx,
		"lets-chat-template.json",
		"lets-chat.png",
	)
	if (err != nil) {
		return fmt.Errorf("Failed creating application template - %s", err.Error())
	}
	err = createAppTemplateOrcs(
		ctx,
		"wordpress-template.json",
		"wordpress.png",
	)
	if (err != nil) {
		return fmt.Errorf("Failed creating application template - %s", err.Error())
	}

	// Registering the default crb
	err = registerCrbOrcs(ctx, "shpan-copy-store", fmt.Sprintf("http://%s:%d/shpan-copy-store-api", svc.Spec.ClusterIP, svc.Spec.Ports[0].Port))
	if (err != nil) {
		return err
	}

	// Deploying k8spsb
	err = deployK8sPsbOrcs(ctx)
	if (err != nil) {
		return err
	}
	// Deploying mongo-dsb
	err = deployMongoDsbOrcs(ctx)
	if (err != nil) {
		return err
	}
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

	return nil;
}

func createOrcsServicesConfiguration(
pgService *v1.Service,
siteName string) configuration.StaticConfigurationNode {
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
					Name: "default",
					DatasourceName: "site-db",
					PersistTasks: "true",
				},
			},
		},
	}

	// Register hub stuff
	buildHubServiceConfiguration(&rootConfNode, pgService)
	buildSiteServiceConfiguration(&rootConfNode, pgService, siteName)

	return rootConfNode

}

func getOrcsServiceUrl(ctx *cmd.DeployerContext) (error, string) {
	orcsService, err := ctx.Client.GetServiceInfo("orcs")
	if (err != nil) {
		return fmt.Errorf("Failed locating orcs service service - %s", err.Error()), ""
	}

	var serviceUrl string;
	if (orcsService.Spec.Type == v1.ServiceTypeLoadBalancer) {
		serviceUrl =
			"http://" + extractLoadBalancerAddress(orcsService.Status.LoadBalancer) + ":80"
	} else if (orcsService.Spec.Type == v1.ServiceTypeNodePort) {
		serviceUrl = "http://" + ctx.ClusterIp + ":" +
			strconv.Itoa(orcsService.Spec.Ports[0].NodePort)
	} else {
		return errors.New("Failed locating public Url when deploying orcs"), ""
	}
	return nil, serviceUrl

}

func buildHubServiceConfiguration(rootConfig *configuration.StaticConfigurationNode, pgService *v1.Service) {
	serviceConfigConfNode := rootConfig.Children["serviceconfig"]
	dataSourceConfNode := rootConfig.Children["datasource"]
	blobStoreConfNode := rootConfig.Children["blobstore"]

	hubDsConfNode := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server: pgService.Spec.ClusterIP,
			Port: strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName: "postgres",
			DbUser: "postgres",
			DbPassword: "nazsecret",
			DbSchema: "hub_repository",
			MaxConnections: "2",
		},
	}
	blobStoreDbConf := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server: pgService.Spec.ClusterIP,
			Port: strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName: "postgres",
			DbUser: "postgres",
			DbPassword: "nazsecret",
			DbSchema: "central_blobstore",
			MaxConnections: "2",
		},
	}

	dataSourceConfNode.Children["hub-db"] = hubDsConfNode
	blobStoreConfNode.Children["image-store"] = blobStoreDbConf

	serviceConfigConfNode.Children["hub"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "hub",
			Route: "http://localhost:8080/hub-api",
			DataSourceConfig: map[string]configuration.DataSourceConfig{
				"hub-db": {
					MaxConnections:2,
				},
			},
			GlobalLoggingConfig: "DEBUG",
		},
	}

	serviceConfigConfNode.Children["hub-web"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "hub-web",
			Route: "http://localhost:8080/hub-web-api",
			BlobstoreConfig: map[string]configuration.DataSourceConfig{
				"image-store": {
					MaxConnections:2,
				},
			},
			GlobalLoggingConfig: "DEBUG",
		},
	}
}

func addQueue(rootConfig *configuration.StaticConfigurationNode, queueName string) {
	rootConfig.Children["queue"].Children[queueName] = configuration.StaticConfigurationNode{
		Data: configuration.PersistentQueueConfiguration{
			DestinationType: "QUEUE",
			QueueName: queueName,
			MemoryBufferMaxMessages: "1000",
			SecondsToSleepBetweenMessageRetries: "2",
			MaxRetries: "2",
		},
	}
}
func buildSiteServiceConfiguration(rootConfig *configuration.StaticConfigurationNode, pgService *v1.Service, siteName string) {
	serviceConfigConfNode := rootConfig.Children["serviceconfig"]
	dataSourceConfNode := rootConfig.Children["datasource"]
	blobStoreConfNode := rootConfig.Children["blobstore"]

	addQueue(rootConfig, "deployed-application-events")
	addQueue(rootConfig, "pending-deployed-application-events")
	addQueue(rootConfig, "application-copy-events")
	addQueue(rootConfig, "pending-application-copy-events")

	siteDsConfNode := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server: pgService.Spec.ClusterIP,
			Port: strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName: "postgres",
			DbUser: "postgres",
			DbPassword: "nazsecret",
			DbSchema: "site_repository",
			MaxConnections: "2",
		},
	}
	protectionDsConfNode := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server: pgService.Spec.ClusterIP,
			Port: strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName: "postgres",
			DbUser: "postgres",
			DbPassword: "nazsecret",
			DbSchema: "protection_repository",
			MaxConnections: "2",
		},
	}

	blobStoreDbConf := configuration.StaticConfigurationNode{
		Data: configuration.StandalonePostgresDatasourceConfiguration{
			Server: pgService.Spec.ClusterIP,
			Port: strconv.Itoa(pgService.Spec.Ports[0].Port),
			DatabaseName: "postgres",
			DbUser: "postgres",
			DbPassword: "nazsecret",
			DbSchema: "central_blobstore",
			MaxConnections: "2",
		},
	}

	dataSourceConfNode.Children["site-db"] = siteDsConfNode
	dataSourceConfNode.Children["protection-db"] = protectionDsConfNode
	blobStoreConfNode.Children["copy-store"] = blobStoreDbConf

	serviceConfigConfNode.Children["site"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "site",
			Route: "http://localhost:8080/site-api",
			DataSourceConfig: map[string]configuration.DataSourceConfig{
				"site-db": {
					MaxConnections:2,
				},
			},
			InputQueueConfig: map[string]configuration.InputQueueConfig{
				"deployed-application-events": {
					NumberOfConsumers:5,
					LogInDebug:true,
				},
				"pending-deployed-application-events": {
					NumberOfConsumers:1,
					LogInDebug:true,
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
			Parameters: map[string]string{
				"site-name": siteName,
				"location": "{" +
					"\"latitude\":32.1792126," +
					"\"longitude\":34.9005128," +
					"\"name\":\"Israel\"," +
					"\"properties\":{}}",
			},
			GlobalLoggingConfig: "DEBUG",
		},
	}

	// Adding protection service
	serviceConfigConfNode.Children["protection"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "protection",
			Route: "http://localhost:8080/protection-api",
			DataSourceConfig: map[string]configuration.DataSourceConfig{
				"protection-db": {
					MaxConnections:2,
				},
			},
			InputQueueConfig: map[string]configuration.InputQueueConfig{
				"application-copy-events": {
					NumberOfConsumers:5,
					LogInDebug:true,
				},
				"pending-application-copy-events": {
					NumberOfConsumers:1,
					LogInDebug:true,
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
			Parameters: map[string]string{
				"site-name": "minikube",
				"location": "{" +
					"\"latitude\":32.1792126," +
					"\"longitude\":34.9005128," +
					"\"name\":\"Israel\"," +
					"\"properties\":{}}",
			},
			GlobalLoggingConfig: "DEBUG",
		},
	}

	// Adding shpan-copy-store
	serviceConfigConfNode.Children["shpan-copy-store"] = configuration.StaticConfigurationNode{
		Data: configuration.ServiceConfig{
			ServiceURI: "shpan-copy-store",
			Route: "http://localhost:8080/shpan-copy-store-api",
			BlobstoreConfig: map[string]configuration.DataSourceConfig{
				"copy-store": {
					MaxConnections: 2,
				},
			},
			GlobalLoggingConfig: "DEBUG",
		},
	}
}

func deployK8sPsbOrcs(ctx *cmd.DeployerContext) (error) {
	fmt.Println("Deploying k8s-psb")
	// Verifying site is registered
	err, siteId := getSiteIdOrcs(ctx, "site")
	if (err != nil) {
		return err
	}

	svc, err := deployService(
		ctx.Client,
		"k8spsb",
		ctx.DeploymentType,
		false,
		true,
		[]v1.EnvVar{
			{Name:"K8S_USERNAME", Value: ctx.Client.UserName},
			{Name:"K8S_PASSWORD", Value: ctx.Client.Password},
			{Name:"NAZ_NAMESPACE", Value: ctx.Client.Namespace},
		},
		80,
		8080,
		[]v1.ServicePort{},
		"ocopea/go-k8s-psb",
		"psb",
		ctx.ClusterIp)

	if (err != nil) {
		return err
	}

	serviceUrl := "http://" + svc.Spec.ClusterIP + "/k8spsb-api";
	fmt.Printf("k8spsb deployed on ip %s\n", serviceUrl)


	// Registering PSB on site
	fmt.Println("Registering cf-psb on site")
	err = postCommandOrcs(
		ctx,
		"hub-web",
		"add-psb",
		UICommandAddPsb{
			PsbUrn:"k8spsb",
			PsbUrl: serviceUrl,
			SiteId: siteId,
		},
		http.StatusNoContent)

	if (err != nil) {
		return err
	}
	fmt.Println("cf-psb registered successfully on site")

	// Adding docker artifact registry
	err = addDockerArtifactRegistryOrcs(ctx, siteId)

	return err;
}

func deployMongoDsbOrcs(ctx *cmd.DeployerContext) (error) {

	fmt.Println("Deploying mongo-dsb")
	// Verifying site is registered on hub
	svc, err := deployService(
		ctx.Client,
		"mongo-k8s-dsb",
		ctx.DeploymentType,
		false,
		true,
		[]v1.EnvVar{
			{Name:"K8S_USERNAME", Value: ctx.Client.UserName},
			{Name:"K8S_PASSWORD", Value: ctx.Client.Password},
			{Name:"NAZ_NAMESPACE", Value: ctx.Client.Namespace},
			{Name:"PORT", Value: "8080"},
			{Name:"HOST", Value: "0.0.0.0"},
		},
		80,
		8080,
		[]v1.ServicePort{},
		"ocopea/mongo-k8s-dsb",
		"dsb",
		ctx.ClusterIp,
	)

	if (err != nil) {
		return err
	}
	fmt.Println("mongo-dsb has been deployed successfully")

	// Adding the dsb to the site
	err = registerDsbOrcs(ctx, "mongo-k8s-dsb", "http://" + svc.Spec.ClusterIP + "/dsb")
	if (err != nil) {
		return err
	}

	return err;
}

func deployK8sDsbOrcs(ctx *cmd.DeployerContext) (error) {
	// Verifying site is registered on hub
	svc, err := deployService(
		ctx.Client,
		"k8sdsb",
		ctx.DeploymentType,
		false,
		true,
		[]v1.EnvVar{
			{Name:"K8S_USERNAME", Value: ctx.Client.UserName},
			{Name:"K8S_PASSWORD", Value: ctx.Client.Password},
			{Name:"NAZ_NAMESPACE", Value: ctx.Client.Namespace},
			{Name:"PORT", Value: "8080"},
			{Name:"HOST", Value: "0.0.0.0"},
		},
		80,
		8080,
		[]v1.ServicePort{},
		"ocopea/k8s-dsb",
		"dsb",
		ctx.ClusterIp,
	)

	if (err != nil) {
		return err
	}

	// Adding the dsb to the site
	err = registerDsbOrcs(ctx, "k8s-dsb", "http://" + svc.Spec.ClusterIP + "/dsb")
	if (err != nil) {
		return err
	}

	return err;

}

func deployK8sVolumeDsbOrcs(ctx *cmd.DeployerContext) (error) {
	// Verifying site is registered on hub
	svc, err := deployService(
		ctx.Client,
		"k8svolumedsb",
		ctx.DeploymentType,
		false,
		true,
		[]v1.EnvVar{
			{Name:"K8S_USERNAME", Value: ctx.Client.UserName},
			{Name:"K8S_PASSWORD", Value: ctx.Client.Password},
			{Name:"NAZ_NAMESPACE", Value: ctx.Client.Namespace},
			{Name:"PORT", Value: "8080"},
			{Name:"HOST", Value: "0.0.0.0"},
		},
		80,
		8080,
		[]v1.ServicePort{},
		"ocopea/k8s-volume-dsb",
		"dsb",
		ctx.ClusterIp,
	)

	if (err != nil) {
		return err
	}

	// Adding the dsb to the site
	err = registerDsbOrcs(ctx, "k8s-volume-dsb", "http://" + svc.Spec.ClusterIP + "/dsb")
	if (err != nil) {
		return err
	}

	return err;

}

func registerDsbOrcs(ctx *cmd.DeployerContext, dsbUrn string, dsbUrl string) error {
	fmt.Printf("Registering DSB %s on site using url %s\n", dsbUrn, dsbUrl)

	err, siteId := getSiteIdOrcs(ctx, "site")
	if (err != nil) {
		return err
	}
	err = postCommandOrcs(
		ctx,
		"hub-web",
		"add-dsb",
		UICommandAddDsb{
			DsbUrn:dsbUrn,
			DsbUrl: dsbUrl,
			SiteId: siteId,
		},
		http.StatusOK)

	if err == nil {
		fmt.Printf("DSB %s has been registered on site successfully\n", dsbUrn)
	}
	return err

}

func registerCrbOrcs(ctx *cmd.DeployerContext, crbUrn string, crbUrl string) error {
	fmt.Printf("Registering crb %s on site using url %s\n", crbUrn, crbUrl)
	err, siteId := getSiteIdOrcs(ctx, "site")
	if (err != nil) {
		return err
	}
	err = postCommandOrcs(
		ctx,
		"hub-web",
		"add-crb",
		UICommandAddCrb{
			CrbUrn:crbUrn,
			CrbUrl: crbUrl,
			SiteId: siteId,
		},
		http.StatusNoContent)

	if (err == nil) {
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
	err, orcsServiceUrl := getOrcsServiceUrl(ctx)
	if (err != nil) {
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

	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth("shpandrak", "1234")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Failed executing command %s on %s - %s", commandName, svcUrn, err.Error())
	}

	jb, err := json.MarshalIndent(jsonBody, "", "    ")
	if (err == nil) {
		log.Printf("Post to %s\n%s\n returned %s\n", urlToPost, string(jb), resp.Status)
	}
	if (resp.StatusCode != expectedStatusCode) {
		return fmt.Errorf("Failed executing command %s on %s - %s", commandName, svcUrn, resp.Status)
	}

	return nil;
}

func addDockerArtifactRegistryOrcs(ctx *cmd.DeployerContext, siteId string) error {

	fmt.Println("configuring the docker hub as the sites default artifact registry")
	err := postCommandOrcs(
		ctx,
		"hub-web",
		"add-docker-artifact-registry",
		UICommandAddDockerArtifactRegistry{
			Name: "shpanRegistry",
			Url: "https://registry.hub.docker.com",
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
		Name: "deploy-site",
		Executor: deploySite,
	}

	cmd.FlagSet = flag.NewFlagSet(cmd.Name, flag.ExitOnError)

	deploySiteArgs = &deploySiteArgsBag{
		siteName : cmd.FlagSet.String("site-name", "", "Site Name"),
		cleanup : cmd.FlagSet.Bool("cleanup", false, "Cleanup Namespace before deploying"),
	}
	return cmd
}

func main() {

	f, err := os.OpenFile("deployer.log", os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)

	// define all the supported commands
	deployerCommands := []*cmd.DeployerCommand{
		defineDeploySiteCommand(),
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
	var replicas int = 1;
	// Building rc spec

	spec := v1.ReplicationControllerSpec{}
	spec.Replicas = &replicas
	spec.Selector = make(map[string]string)
	spec.Selector["app"] = serviceName
	spec.Template = &v1.PodTemplateSpec{}
	spec.Template.ObjectMeta = v1.ObjectMeta{}
	spec.Template.ObjectMeta.Labels = make(map[string]string)
	spec.Template.ObjectMeta.Labels["app"] = serviceName

	if (nazKind != "") {
		spec.Template.ObjectMeta.Labels["nazKind"] = nazKind
	}

	containerSpec := v1.Container{}
	containerSpec.Name = serviceName

	containerSpec.Image = imageName;
	containerSpec.ImagePullPolicy = v1.PullIfNotPresent
	containerSpec.Ports = []v1.ContainerPort{{ContainerPort:8080}}
	containerSpec.Env = []v1.EnvVar{{Name:"NAZ_DEPLOYMENT_TYPE", Value: deploymentType}}
	containers := []v1.Container{containerSpec}

	spec.Template.Spec = v1.PodSpec{}
	spec.Template.Spec.Containers = containers

	// Create a replicationController object for running the app
	rc := &v1.ReplicationController{}
	rc.Name = serviceName;
	rc.Labels = make(map[string]string)
	if (nazKind != "") {
		rc.Labels["nazKind"] = nazKind
	}
	rc.Spec = spec
	return rc, nil;

}
func createK8SServiceStruct(serviceName string, deploymentType string, exposePublic bool, mainPort int, mainContainerPort int, additionalPorts []v1.ServicePort) (*v1.Service, error) {
	svc := &v1.Service{}
	svc.Name = serviceName

	var svcType v1.ServiceType
	if (exposePublic) {
		switch deploymentType {
		case "local":
			svcType = v1.ServiceTypeNodePort;
			break;
		case "aws":
		case "gcp":
		default:
			svcType = v1.ServiceTypeLoadBalancer;
			break;

		}
	} else {
		svcType = v1.ServiceTypeClusterIP;
	}
	svc.Spec.Type = svcType;
	svc.Spec.Ports = []v1.ServicePort{{
		Port:mainPort,
		TargetPort: types.NewIntOrStringFromInt(mainContainerPort),
		Protocol:v1.ProtocolTCP,
		Name:"service-http"}}

	// Appending additional ports where applicable
	for _, currPort := range additionalPorts {
		svc.Spec.Ports =
			append(
				svc.Spec.Ports,
				currPort)
	}

	svc.Spec.Selector = map[string]string{"app": serviceName}

	return svc, nil;

}

func deployService(
client *k8sClient.Client,
serviceName string,
deploymentType string,
exposePublic bool,
force bool,
additionalVars[]v1.EnvVar,
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
	if (err != nil) {
		return nil, err
	}

	rcRequest, err := createReplicationControllerStruct(serviceName, deploymentType, imageName, nazKind);
	if (err != nil) {
		return nil, fmt.Errorf(
			"Failed reading data from replication controller file for %s - %s", serviceName, err.Error());
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

	/*
	rcRequest.Spec.Template.Spec.Containers[0].Env =
		append(
			rcRequest.Spec.Template.Spec.Containers[0].Env,
			v1.EnvVar{Name:"NAZ_ROUTE", Value: "http://" + svc.Spec.ClusterIP})
	*/

	var publicRoute string
	additionalPortsJSON := ""
	if (svc.Spec.Type == v1.ServiceTypeLoadBalancer) {
		for _, currPort := range svc.Spec.Ports {
			if (currPort.Name == "service-http") {
				publicRoute = "http://" + extractLoadBalancerAddress(svc.Status.LoadBalancer) + ":" + strconv.Itoa(currPort.Port)
			} else {
				if (additionalPortsJSON == "") {
					additionalPortsJSON = "{"
				} else {
					additionalPortsJSON += ","
				}
				// Adding env var telling the container which port has been mapped to the requested additional port
				additionalPortsJSON += "\"" + currPort.Name + "\":\"" + strconv.Itoa(currPort.Port) + "\"";
			}
		}
		if publicRoute == "" {
			return nil, fmt.Errorf("Failed assigning public port to service %s", serviceName);
		}

	} else if (svc.Spec.Type == v1.ServiceTypeNodePort) {
		for _, currPort := range svc.Spec.Ports {
			if (currPort.Name == "service-http") {
				publicRoute = "http://" + clusterIp + ":" + strconv.Itoa(currPort.NodePort)
			} else {
				// Adding env var telling the container which port has been mapped to the requested additional port
				if (additionalPortsJSON == "") {
					additionalPortsJSON = "{"
				} else {
					additionalPortsJSON += ","
				}
				// Adding env var telling the container which port has been mapped to the requested additional port
				additionalPortsJSON += "\"" + currPort.Name + "\":\"" + strconv.Itoa(currPort.NodePort) + "\"";
			}
		}
		if publicRoute == "" {
			return nil, fmt.Errorf("Failed assigning NodePort to service %s", serviceName);
		}
	}

	if (publicRoute != "") {
		fmt.Printf("for service %s - naz route - %s\n", serviceName, publicRoute)
		rcRequest.Spec.Template.Spec.Containers[0].Env =
			append(
				rcRequest.Spec.Template.Spec.Containers[0].Env,
				v1.EnvVar{Name:"NAZ_PUBLIC_ROUTE", Value: publicRoute})
	}

	if (additionalPortsJSON != "") {
		additionalPortsJSON += "}"
		log.Printf("Additional ports %s\n", additionalPortsJSON)
		rcRequest.Spec.Template.Spec.Containers[0].Env = append(
			rcRequest.Spec.Template.Spec.Containers[0].Env,
			v1.EnvVar{Name:"NAZ_PORTS", Value:additionalPortsJSON})

	}

	if (deploymentType == "local") {
		rcRequest.Spec.Template.Spec.Containers[0].Env =
			append(
				rcRequest.Spec.Template.Spec.Containers[0].Env,
				v1.EnvVar{Name:"LOCAL_CLUSTER_IP", Value: clusterIp})

	}

	_, err = deployK8SReplicationController(client, serviceName, rcRequest, force)
	if (err != nil) {
		return nil, fmt.Errorf("Failed creating replication controller for %s - %s", serviceName, err.Error());
	}

	return svc, nil

}

func deployK8SReplicationController(client *k8sClient.Client, serviceName string, rc *v1.ReplicationController, force bool) (*v1.ReplicationController, error) {
	rc, err := client.CreateReplicationController(rc, force)
	if (err != nil) {
		return nil, fmt.Errorf("Failed creating k8s replication controller for %s - %s", serviceName, err.Error());
	}
	log.Printf("%s replication controller has been deployed successfully\n", rc.Name)

	// Now waiting for replication controller to schedule a single replication
	for retries := 60; rc.Status.Replicas == 0 && retries > 0; retries-- {
		rc, err = client.GetReplicationControllerInfo(rc.Name)
		if (err != nil) {
			return nil, fmt.Errorf("Failed getting k8s replication controller for %s - %s", serviceName, err.Error());
		}
		time.Sleep(1 * time.Second)
	}

	if rc.Status.Replicas == 0 {
		return  nil, fmt.Errorf("Replication controller %s failed creating replicas after waiting for 60 seconds", rc.Name)
	}

	log.Printf("%s replication controller has been deployed successfully and replicas already been observed\n", rc.Name)

	// Now we want to see that we have a pod scheduled by the rc
	rcPod, err := waitForReplicationControllerPodToSchedule(client, rc)
	if err != nil {
		return nil, err
	}

	log.Printf("pod %s for replication controller %s has been scheduled and observed\n", rcPod.Name, rc.Name)

	// Now we're wating to the scheduled pod to actually start with a running container
	err = waitForPodToBeRunning(client, rcPod)
	if err != nil {
		return nil, err
	}
	log.Printf("pod %s for replication controller %s has been started successfuly\n", rcPod.Name, rc.Name)

	return rc, nil

}

func waitForPodToBeRunning(client *k8sClient.Client, pod *v1.Pod) error {
	var err error = nil
	// now we want to see that the stupid pod is really starting!
	for retries := 180; pod.Status.Phase != "Running" && retries > 0; retries-- {
		pod, err = client.GetPodInfo(pod.Name)
		if (err != nil) {
			return fmt.Errorf("Failed getting pod %s while waiting for it to be a sweetheart and run - %s", pod.Name, err.Error());
		}
		time.Sleep(1 * time.Second)
	}

	if (pod.Status.Phase == "Running") {
		log.Printf("pod %s is now running, yey\n", pod.Name)
		return nil
	} else {

		additionalErrorMessage := ""
		// Enriching error message
		if (pod.Status.ContainerStatuses != nil &&
		len(pod.Status.ContainerStatuses) > 0) {
			if pod.Status.ContainerStatuses[0].State.Waiting != nil {
				additionalErrorMessage +=
					fmt.Sprintf(
						". container still waiting. reason:%s; message:%s",
						pod.Status.ContainerStatuses[0].State.Waiting.Reason,
						pod.Status.ContainerStatuses[0].State.Waiting.Message)
			} else if (pod.Status.ContainerStatuses[0].State.Terminated != nil) {
				additionalErrorMessage +=
					fmt.Sprintf(
						". container terminated. reason:%s; message:%s; exit code:%d",
						pod.Status.ContainerStatuses[0].State.Terminated.Reason,
						pod.Status.ContainerStatuses[0].State.Terminated.Message,
						pod.Status.ContainerStatuses[0].State.Terminated.ExitCode,
					)
			}
		}
		return fmt.Errorf("Pod %s did not start after 10 freakin' minutes and found in phase %s%s", pod.Name, pod.Status.Phase, additionalErrorMessage)
	}
}


func waitForReplicationControllerPodToSchedule(client *k8sClient.Client, rc *v1.ReplicationController) (*v1.Pod, error) {
	log.Printf("searching for pods scheduled by replication controller %s\n", rc.Name)
	for retries := 60; retries > 0; retries-- {

		// Searching for the single pod scheduled by the rc
		rcPods, err := client.ListPodsInfo(rc.Spec.Selector)
		if (err != nil) {
			return nil, fmt.Errorf("Failed searching for pods scheduled for rc %s - %s", rc.Name, err.Error());
		}
		if (len(rcPods) == 0) {
			log.Printf("Could not yet find pods associated with replication controller %s\n", rc.Name)
		} else {
			thePod := rcPods[0]
			log.Printf("Found Pod %s, scheduled for rc %s\n", thePod.Name, rc.Name)
			return thePod, nil;
		}
		time.Sleep(1 * time.Second)
	}

	return nil, fmt.Errorf("Failed finding pod scheduled for replication controller %s after waiting for 60 seconds", rc.Name);


}
func deployK8SService(client *k8sClient.Client, svc *v1.Service, force bool) (*v1.Service, error) {
	svc, err := client.CreateService(svc, force)
	if (err != nil) {
		return nil, err;
	}

	svc, err = client.WaitForServiceToStart(svc.Name, 100, 3 * time.Second)
	if (err != nil) {
		return svc, err;
	}
	log.Printf("Service %s deployed successfully\n", svc.Name)
	return svc, nil

}

func createAppTemplateOrcs(ctx *cmd.DeployerContext, templatePath string, iconPath string) error {
	fmt.Printf("creating the application template using %s, icon:%s\n", templatePath, iconPath)
	err, orcsServiceUrl := getOrcsServiceUrl(ctx)
	if (err != nil) {
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

	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth("shpandrak", "1234")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return fmt.Errorf("Failed posting new app template - %s", err.Error())
	}

	if (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict) {
		return fmt.Errorf("Failed creating app template - %s", resp.Status)
	}

	defer resp.Body.Close()
	btt, err := ioutil.ReadAll(resp.Body)
	if (err != nil) {
		log.Println("Whatever - failed reading create template response " + err.Error())
	}

	if (resp.StatusCode != http.StatusConflict) {
		appTemplateId := strings.Replace(string(btt), "\"", "", 2)

		f, err := os.Open(iconPath)
		if err != nil {
			log.Println("Whatever - failed reading icon path - " + err.Error())
		}
		postIconUrl := orcsServiceUrl + "/hub-web-api/images/app-template/" + appTemplateId
		log.Println("Posting icon to " + postIconUrl)
		reqIcon, err := http.NewRequest(
			"POST",
			postIconUrl,
			f)

		if err != nil {
			log.Println("Whatever - failed reading create post for icon - " + err.Error())
		}
		reqIcon.Header.Add("Content-Type", "application/octet-stream")
		reqIcon.SetBasicAuth("shpandrak", "1234")

		_, err = http.DefaultClient.Do(reqIcon)
		if err != nil {
			log.Println("Whatever - failed posting app template icon - " + err.Error())
		}
	}
	fmt.Printf("successfully created application template using %s, icon:%s\n", templatePath, iconPath)
	return nil;
}

func getSiteIdOrcs(ctx *cmd.DeployerContext, siteUrn string) (error, string) {
	err, orcsUrl := getOrcsServiceUrl(ctx)
	if (err != nil) {
		return err, ""
	}
	req, err := http.NewRequest(
		"GET",
		orcsUrl + "/hub-web-api/site",
		nil)
	if err != nil {
		return fmt.Errorf("Failed createing app template req - %s", err.Error()), ""
	}

	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth("shpandrak", "1234")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return fmt.Errorf("Failed posting new app template - %s", err.Error()), ""
	}

	if (resp.StatusCode != http.StatusOK) {
		return fmt.Errorf("Failed creating app template - %s", resp.Status), ""
	}

	var sitesArray []UISite;
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&sitesArray)
	if err != nil {
		return fmt.Errorf("Failed decoding sits array - %s", err.Error()), ""
	}

	//todo: only do on debug - is there a go shit to do that? if debug?
	str, err := json.Marshal(sitesArray)
	if (err != nil) {
		log.Println(err)
	}
	log.Printf(string(str))

	for _, currSite := range sitesArray {
		if (currSite.Urn == siteUrn) {
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
			return nil;
		} else {
			log.Printf("service %s is not ready yet - %s\n", serviceEndpoint, err.Error())
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("service endpoint %s failed starting in a timely fasion - %s", serviceEndpoint, err.Error());
}

func verifyOrcsServiceHasStarted(ctx *cmd.DeployerContext, serviceEndpoint string) error {
	err, orcsServiceUrl := getOrcsServiceUrl(ctx)
	if (err != nil) {
		return fmt.Errorf("Failed getting orcs service url - %s", err.Error())
	}

	stateUrl := orcsServiceUrl + "/" + serviceEndpoint + "/state"
	log.Printf("Verifying service %s is running by getting it's status from %s\n", serviceEndpoint, stateUrl)

	req, err := http.NewRequest("GET", stateUrl, nil)
	if err != nil {
		return fmt.Errorf("failed creating get request on %s - %s", stateUrl, err.Error())
	}
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth("shpandrak", "1234")

	resp, err := http.DefaultClient.Do(req)
	if (err != nil) {
		return err
	}
	defer resp.Body.Close()

	all, err := ioutil.ReadAll(resp.Body)
	if (err != nil) {
		return err
	}
	log.Println(string(all))

	var dat map[string]interface{}
	json.Unmarshal([]byte(all), &dat)
	stateObj := dat["state"]
	if (stateObj == nil) {
		return fmt.Errorf("service state endpint for service %s, didn't contain a \"state\" field", serviceEndpoint)
	}
	state := stateObj.(string)
	fmt.Println(state)
	if strings.Compare(state, "RUNNING") != 0 {
		return fmt.Errorf("site is not running but %s", state)
	}
	return nil
}

func extractLoadBalancerAddress(loadBalancerStatus v1.LoadBalancerStatus) string {
	if (len(loadBalancerStatus.Ingress[0].IP) > 0) {
		return loadBalancerStatus.Ingress[0].IP
	}
	if (len(loadBalancerStatus.Ingress[0].Hostname) > 0) {
		return loadBalancerStatus.Ingress[0].Hostname
	}
	return ""
}
