// This Service implements Ocopea PaaS Service Broker (PSB) API for running apps on top of Kubernetes
// See PSB API Reference at [todo]

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net/http"
	kubernetesClient "ocopea/kubernetes/client"
	"ocopea/kubernetes/client/types"
	"ocopea/kubernetes/client/v1"
	"os"
	"strconv"
	"strings"
	"time"
)

var upgrader = websocket.Upgrader{} // use default options

type PSBSpaceDTO struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

type psbInfo struct {
	Name                  string `json:"name"`
	Version               string `json:"version"`
	Type                  string `json:"type"`
	Description           string `json:"description"`
	AppServiceIdMaxLength int    `json:"appServiceIdMaxLength"`
}

type appInstanceInfo struct {
	Name          string            `json:"name"`
	Status        string            `json:"status"`
	StatusMessage string            `json:"statusMessage"`
	Instances     int               `json:"instances"`
	PsbMetrics    map[string]string `json:"psbMetrics"`
	EntryPointURL string            `json:"entryPointURL"`
}

type stateInfo struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type PSBBindPortDTO struct {
	Protocol    string `json:"protocol"`
	Destination string `json:"destination"`
	Port        int    `json:"port"`
}

type PSBServiceBindingInfoDTO struct {
	ServiceName string            `json:"serviceName"`
	ServiceId   string            `json:"serviceId"`
	Plan        string            `json:"plan"`
	BindInfo    map[string]string `json:"bindInfo"`
	Ports       []PSBBindPortDTO  `json:"ports"`
}

type PSBLogsWebSocketDTO struct {
	Address       string `json:"address"`
	Serialization string `json:"serialization"`
}

type deployAppServiceManifestDTO struct {
	AppServiceId               string                                `json:"appServiceId"`
	Space                      string                                `json:"space"`
	ImageName                  string                                `json:"imageName"`
	ImageVersion               string                                `json:"imageVersion"`
	EnvironmentVariables       map[string]string                     `json:"environmentVariables"`
	ArtifactRegistryType       string                                `json:"artifactRegistryType"`
	ArtifactRegistryParameters map[string]string                     `json:"artifactRegistryParameters"`
	PsbSettings                map[string]string                     `json:"psbSettings"`
	Route                      string                                `json:"route"`
	ExposedPorts               []int                                 `json:"exposedPorts"`
	HttpPort                   int                                   `json:"httpPort"`
	ServiceBindings            map[string][]PSBServiceBindingInfoDTO `json:"serviceBindings"`
}
type PSBLogMessageDTO struct {
	Message     string `json:"message"`
	Timestamp   int64  `json:"timestamp"`
	MessageType string `json:"messageType"` // ("out"/"err")
	ServiceId   string `json:"serviceId"`
}

type deployError struct {
	httpStatusCode int
	message        string
}

type requestContext struct {
	r      *http.Request
	w      *http.ResponseWriter
	partId string
	done   chan bool
}

var gPSBInfo = psbInfo{
	Name:                  "k8s-mini",
	Version:               "0.1",
	Type:                  "k8s",
	Description:           "K8S Mini PaaS",
	AppServiceIdMaxLength: 24,
}
var kClient *kubernetesClient.Client
var deploymentType string
var gLocalClusterIp string
var gInClusterServiceAddr string

func (e deployError) Error() string {
	return e.message
}

func handlePSBInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.Encode(gPSBInfo)
	} else {
		w.WriteHeader(404)
	}
}

func handleAppServiceInfo(w http.ResponseWriter, r *http.Request) *deployError {
	if r.Method == "GET" {
		vars := mux.Vars(r)

		w.Header().Set("Content-Type", "application/json")
		appUniqueName := vars["appServiceId"]
		if appUniqueName == "" {
			return &deployError{
				httpStatusCode: http.StatusBadRequest,
				message:        "missing appServiceId path param",
			}
		}
		var serviceURL string = ""
		var status string
		var statusMessage string

		// Returning 404 if none exist
		exists, _ := kClient.CheckServiceExists(appUniqueName)
		if !exists {
			return &deployError{
				httpStatusCode: http.StatusNotFound,
				message:        "could not find service with id " + appUniqueName,
			}
		} else {

			isReady, svc, err := kClient.TestService(appUniqueName)
			if err != nil {
				status = "error"
				statusMessage = err.Error()
			} else if !isReady {
				status = "starting"
			} else {
				status = "running"
				if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
					//todo:2: read correct port from service
					serviceURL = "http://" + extractLoadBalancerAddress(svc.Status.LoadBalancer)
				} else if svc.Spec.Type == v1.ServiceTypeNodePort {
					serviceURL = "http://" + gLocalClusterIp + ":" + strconv.Itoa(svc.Spec.Ports[0].NodePort)
				}
				if serviceURL == "" {
					fmt.Printf("Service %s is shit and has no URL\n", svc.Name)
				}
			}

			var info = appInstanceInfo{
				Name:          vars["appServiceId"],
				Status:        status,
				StatusMessage: statusMessage,
				Instances:     1,
				EntryPointURL: serviceURL,
			}

			enc := json.NewEncoder(w)
			enc.Encode(info)
		}
	} else if r.Method == "DELETE" {
		vars := mux.Vars(r)
		w.Header().Set("Content-Type", "application/json")
		appUniqueName := vars["appServiceId"]
		if appUniqueName == "" {
			return &deployError{
				httpStatusCode: http.StatusBadRequest,
				message:        "missing appServiceId path param",
			}
		}

		// Returning 404 if none exist
		exists, _ := kClient.CheckServiceExists(appUniqueName)
		if !exists {
			return &deployError{
				httpStatusCode: http.StatusNotFound,
				message:        "Service not found for id " + appUniqueName,
			}
		}

		// In order to delete a service we need to delete both replication controller and service
		err := kClient.DeleteReplicationController(appUniqueName)
		if err != nil {
			return &deployError{
				httpStatusCode: http.StatusInternalServerError,
				message:        fmt.Sprintf("Failed deleting replication controller %s", appUniqueName),
			}
		}
		err = kClient.DeleteService(appUniqueName)
		if err != nil {
			return &deployError{
				httpStatusCode: http.StatusInternalServerError,
				message:        fmt.Sprintf("Failed deleting replication service %s", appUniqueName),
			}
		}
	} else {
		return &deployError{
			httpStatusCode: http.StatusUnsupportedMediaType,
			message:        "Unsupported media type " + r.Method,
		}
	}
	return nil
}
func handleAppServiceLogs(w http.ResponseWriter, r *http.Request) *deployError {
	if r.Method == "GET" {

		w.Header().Set("Content-Type", "application/json")
		//vars := mux.Vars(r)
		//appUniqueName := vars["appServiceId"]

		enc := json.NewEncoder(w)

		var wsAddress string
		if gInClusterServiceAddr != "" {
			wsAddress = fmt.Sprintf("ws://%s%s/data", gInClusterServiceAddr, r.RequestURI)
		} else {
			wsAddress = fmt.Sprintf("ws://%s%s/data", r.RemoteAddr, r.RequestURI)
		}
		log.Printf("ws address: %s", wsAddress)
		w.WriteHeader(http.StatusOK)
		err := enc.Encode(PSBLogsWebSocketDTO{
			Address:       wsAddress,
			Serialization: "json",
		})
		if err != nil {
			return &deployError{httpStatusCode: http.StatusInternalServerError, message: "Failed decoding log info " + err.Error()}
		}

	} else {
		w.WriteHeader(http.StatusUnsupportedMediaType)
	}
	return nil
}

func handleAppServiceLogsData(w http.ResponseWriter, r *http.Request) *deployError {
	messagesChannel := make(chan string, 50)
	wsClosedChannel := make(chan bool, 1)

	vars := mux.Vars(r)
	appUniqueName := vars["appServiceId"]
	if appUniqueName == "" {
		return &deployError{
			message:        "invalid empty appServiceId parameter",
			httpStatusCode: 400,
		}
	}
	log.Printf("Web socket opened for service %s", appUniqueName)

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade error:", err)
		return &deployError{
			message:        "upgrade:" + err.Error(),
			httpStatusCode: 500,
		}
	}
	c.SetCloseHandler(func(code int, text string) error {
		log.Printf("Web socket for service %s closed with code %d and text %s", appUniqueName, code, text)
		wsClosedChannel <- true
		message := []byte{}
		if code != websocket.CloseNoStatusReceived {
			message = websocket.FormatCloseMessage(code, "")
		}
		c.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second))

		return nil
	})

	wsConnectionOpen := true

	// Getting pods involved in the app
	pods := []*v1.Pod{}
	for len(pods) == 0 && wsConnectionOpen {

		select {
		case _ = <-wsClosedChannel:
			wsConnectionOpen = false
		default:
			pods, err = kClient.ListPodsInfo(map[string]string{
				"app": appUniqueName,
			})

			if err != nil {
				log.Print("error getting pods for logs:", err)
				return &deployError{
					message:        "error getting pods for logs:" + err.Error(),
					httpStatusCode: 500,
				}
			}
			time.Sleep(1 * time.Second)
		}
	}

	// If the connection has been closed while polling for pods we're done here
	if !wsConnectionOpen {
		log.Printf("web socket for service %s closed before being able to find relevant pods", appUniqueName)
		return nil
	}

	closeHandles := []kubernetesClient.CloseHandle{}

	// Now getting the logs for each pod and putting in the channel
	for _, p := range pods {

		// Kicking this in a goroutine so it won't block us here...
		doneFollowingPod := false

		for !doneFollowingPod {
			closeHandle, err := kClient.FollowPodLogs(p.Name, messagesChannel)
			if err != nil {
				log.Printf("retrying following pod %s logs - %s", p.Name, err.Error())
				time.Sleep(time.Second)
			} else {
				closeHandles = append(closeHandles, closeHandle)
				doneFollowingPod = true
			}
		}
	}

	// iterating over the channel, this will stop once the channel is closed due to web-socket closing
	go func() {
		done := false
		for !done {
			select {
			case messageToPrint := <-messagesChannel:

				// Formatting message
				btt, err := json.Marshal(
					PSBLogMessageDTO{
						Message:     messageToPrint,
						MessageType: "out",
						ServiceId:   appUniqueName,
						Timestamp:   (time.Now().UnixNano() / 1000000),
					})
				if err != nil {
					log.Printf("failed marshaling, whaterver: %s", err.Error())
				} else {

					// Sending the message to the socket
					err = c.WriteMessage(websocket.TextMessage, btt)
					if err != nil {
						log.Println("write error:", err)
					}
				}
			case _ = <-wsClosedChannel:
				done = true
			}
		}

		// Closing all close handles
		for _, h := range closeHandles {
			// Close all followers
			go h()
		}
	}()

	// Keeping the ws alive (ping/pong/close)
	for {
		if _, _, err := c.NextReader(); err != nil {
			c.Close()
			break
		}
	}

	return nil
}

func serviceStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		var state = stateInfo{Name: "k8spsb", State: "RUNNING"}
		enc := json.NewEncoder(w)
		enc.Encode(state)
	} else {
		w.WriteHeader(404)
	}
}

func listSpacesHandler(w http.ResponseWriter, r *http.Request) {
	printHandler(r)
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		namespaces, err := kClient.ListNamespaceInfo(nil)
		if err != nil {
			handleServerError(w, &deployError{httpStatusCode: 500, message: err.Error()}, r)
		} else {
			var spacesDto = []PSBSpaceDTO{}
			for _, ns := range namespaces {
				spacesDto = append(spacesDto, PSBSpaceDTO{
					Name: ns.Name,
				})
			}

			enc := json.NewEncoder(w)
			enc.Encode(spacesDto)
		}
	} else {
		w.WriteHeader(http.StatusUnsupportedMediaType)
	}
}

func deployAppHandler(w http.ResponseWriter, r *http.Request) {
	printHandler(r)
	w.Header().Set("Content-Type", "application/json")
	err := deployApp(w, r)
	if err != nil {
		handleServerError(w, err, r)
	}
}

func appServiceInfoHandler(w http.ResponseWriter, r *http.Request) {
	printHandler(r)
	w.Header().Set("Content-Type", "application/json")
	err := handleAppServiceInfo(w, r)
	if err != nil {
		handleServerError(w, err, r)
	}
}
func appServiceLogsHandler(w http.ResponseWriter, r *http.Request) {
	printHandler(r)
	w.Header().Set("Content-Type", "application/json")
	err := handleAppServiceLogs(w, r)
	if err != nil {
		handleServerError(w, err, r)
	}
}

func logsDataHandler(w http.ResponseWriter, r *http.Request) {
	printHandler(r)
	w.Header().Set("Content-Type", "application/json")
	err := handleAppServiceLogsData(w, r)
	if err != nil {
		handleServerError(w, err, r)
	}
}

func getMandatoryHeader(r *http.Request, headerName string) (string, *deployError) {
	var headerValue string = r.Header.Get(headerName)
	if len(headerValue) == 0 {
		return "", &deployError{
			httpStatusCode: http.StatusBadRequest,
			message:        "missing \"" + headerName + "\" header",
		}
	} else {
		return headerValue, nil
	}

}

func deployApp(w http.ResponseWriter, r *http.Request) *deployError {
	// Only supporting post
	if r.Method != "POST" {
		return &deployError{
			httpStatusCode: http.StatusMethodNotAllowed,
			message:        "method " + r.Method + " Not allowed",
		}
	} else {
		// Getting mandatory header content type
		_, uErr := getMandatoryHeader(r, "Content-Type")
		if uErr != nil {
			return uErr
		}
		dec := json.NewDecoder(r.Body)
		var appManifest deployAppServiceManifestDTO
		err := dec.Decode(&appManifest)
		if err != nil {
			return &deployError{httpStatusCode: http.StatusInternalServerError, message: "Failed decoding app manifest"}
		}

		log.Printf("Running %s with image %s version %s on route %s and %d dsb types bindings",
			appManifest.AppServiceId,
			appManifest.ImageName,
			appManifest.ImageVersion,
			appManifest.Route,
			len(appManifest.ServiceBindings))

		envVarFilters := make(map[string]string)
		bindingsVar := v1.EnvVar{Name: "NAZ_MS_API_K8S_BINDINGS"}
		if len(appManifest.ServiceBindings) > 0 {
			//            for dsbName, dsbMap := range appManifest.ServiceBindings {
			//
			//            }
			appBindingEnv, err := json.Marshal(appManifest.ServiceBindings)
			if err != nil {
				return &deployError{httpStatusCode: http.StatusInternalServerError, message: "Failed parsing bindings"}
			}

			strVar := string(appBindingEnv)
			bindingsVar.Value = strVar
			log.Println(strVar)

			// Preparing binding keys to do the replace with env vars
			for _, svcBindingsArr := range appManifest.ServiceBindings {
				for _, svcBinding := range svcBindingsArr {
					for k, v := range svcBinding.BindInfo {
						envVarFilters[fmt.Sprintf("${%s.%s}", svcBinding.ServiceName, k)] = v
					}
				}
			}
		}

		k8sDsbBindings := appManifest.ServiceBindings["k8s-dsb"]
		if k8sDsbBindings != nil {
			log.Println("Found k8s-dsb bindings")
			//for
		}

		appUniqueName := appManifest.AppServiceId

		envVar := []v1.EnvVar{bindingsVar}

		// Filter and apply environment variables
		if appManifest.EnvironmentVariables != nil {
			for k, v := range appManifest.EnvironmentVariables {

				// filter env var values
				filteredValue := v

				for filterKey, fValue := range envVarFilters {
					filteredValue = strings.Replace(filteredValue, filterKey, fValue, -1)
				}

				envVar = append(
					envVar,
					v1.EnvVar{
						Name:  k,
						Value: filteredValue,
					},
				)
			}
		}

		var replicas int = 1
		// Building rc spec

		spec := v1.ReplicationControllerSpec{}
		spec.Replicas = &replicas
		spec.Selector = make(map[string]string)
		spec.Selector["app"] = appUniqueName
		spec.Template = &v1.PodTemplateSpec{}
		spec.Template.ObjectMeta = v1.ObjectMeta{}
		spec.Template.ObjectMeta.Labels = make(map[string]string)
		spec.Template.ObjectMeta.Labels["app"] = appUniqueName
		spec.Template.ObjectMeta.Labels["nazKind"] = "app"

		containerSpec := v1.Container{}
		containerSpec.Name = appUniqueName

		imageName := appManifest.ImageName
		if len(appManifest.ImageVersion) != 0 {
			imageName += ":" + appManifest.ImageVersion
		}
		containerSpec.Image = imageName
		containerSpec.Env = envVar
		//todo: 1) all ports
		//todo: 2) allow no http port at all
		//if (appManifest.HttpPort != nil){
		containerSpec.Ports = []v1.ContainerPort{{ContainerPort: appManifest.HttpPort}}
		//}

		containers := []v1.Container{containerSpec}

		spec.Template.Spec = v1.PodSpec{}
		spec.Template.Spec.Containers = containers

		// Create a replicationController object for running the app
		rc := &v1.ReplicationController{}
		rc.Name = appUniqueName
		rc.Labels = make(map[string]string)
		rc.Labels["nazKind"] = "app"
		rc.Spec = spec

		_, err = kClient.CreateReplicationController(rc, false)
		if err != nil {
			return &deployError{httpStatusCode: http.StatusInternalServerError, message: err.Error()}
		}

		svc := &v1.Service{}
		svc.Name = appUniqueName

		switch deploymentType {
		case "local":
			svc.Spec.Type = v1.ServiceTypeNodePort
			break
		default:
			svc.Spec.Type = v1.ServiceTypeLoadBalancer
			break
		}

		svc.Spec.Ports = []v1.ServicePort{{
			Port:       80,
			TargetPort: types.NewIntOrStringFromInt(appManifest.HttpPort),
			Protocol:   v1.ProtocolTCP,
			Name:       "tcp",
		}}

		svc.Spec.Selector = map[string]string{"app": appUniqueName}

		svc, err = kClient.CreateService(svc, false)
		if err != nil {
			return &deployError{httpStatusCode: http.StatusInternalServerError, message: "failed creating service " + appUniqueName + " : " + err.Error()}
		}

		// Track and print async...
		go PrintService(svc)

		w.WriteHeader(201)
		io.WriteString(w, "{\"status\":0,\"message\":\"Oh Yeah!\"}")
		fmt.Printf("Image %s deployed successfully\n", appManifest.ImageName)

		return nil
	}

}

func PrintService(s *v1.Service) {
	s, err := kClient.WaitForServiceToStart(s.Name, 100, time.Second*3)
	if err != nil {
		fmt.Printf("Ok, like.. whatever.. %s\n", err.Error())
	}

	var serviceURL string
	if s.Spec.Type == v1.ServiceTypeLoadBalancer {
		//todo:2: read correct port from service
		serviceURL = "http://" + extractLoadBalancerAddress(s.Status.LoadBalancer)
	} else if s.Spec.Type == v1.ServiceTypeNodePort {
		serviceURL = "http://$(docker-machine ip):" + strconv.Itoa(s.Spec.Ports[0].NodePort)
	}
	if serviceURL != "" {
		fmt.Printf("Service %s has been deployed, visit hub at:\n%s\n", s.Name, serviceURL)
	}

}

func printHandler(r *http.Request) {
	log.Printf("Incoming: %s %s\n", r.Method, r.URL.Path)
}

func handleServerError(w http.ResponseWriter, err *deployError, r *http.Request) {
	fmt.Printf("Error: %s %s : %s\n", r.Method, r.URL.Path, err.message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.httpStatusCode)
	var msgBytes, e = json.Marshal(err.Error())
	if e != nil {
		io.WriteString(w, "{\"status\":1,\"message\":\"Unknown Error\"}")
	} else {
		io.WriteString(w, "{\"status\":1,\"message\":"+string(msgBytes)+"}")
	}
}

func main() {
	//runtime.GOMAXPROCS(runtime.NumCPU())
	log.Println("hi, I'm here")
	fmt.Println("Yo, here dude!")

	// Parsing flags
	k8sURL := flag.String("url", "https://kubernetes:443", "K8S remote api url")
	k8sNamespace := flag.String("namespace", "ocopea", "K8S namespace to use")
	flag.Parse()

	host, hb := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	port, pb := os.LookupEnv("KUBERNETES_SERVICE_PORT")

	var lcb bool
	gLocalClusterIp, lcb = os.LookupEnv("LOCAL_CLUSTER_IP")

	deploymentType = os.Getenv("NAZ_DEPLOYMENT_TYPE")
	k8sUserName := os.Getenv("K8S_USERNAME")
	k8sPassword := os.Getenv("K8S_PASSWORD")

	if hb && pb {
		*k8sURL = "https://" + host + ":" + port
	}

	nazNS := os.Getenv("NAZ_NAMESPACE")
	if len(nazNS) == 0 {
		nazNS = *k8sNamespace
	}
	fmt.Printf("url %s\nnamespace:%s\n", *k8sURL, nazNS)

	if deploymentType == "local" &&
		(!lcb || len(gLocalClusterIp) == 0) {
		panic("on local deployments LOCAL_CLUSTER_IP must be defined")
	}

	// Building "secure" http client
	var err error
	kClient, err = kubernetesClient.NewClient(
		*k8sURL,
		nazNS,
		k8sUserName,
		k8sPassword,
		"/var/run/secrets/kubernetes.io/serviceaccount/token")

	if err != nil {
		panic(err)
	}

	k8sPsbHost := os.Getenv("K8SPSB_SERVICE_HOST")
	k8sPsbPort := os.Getenv("K8SPSB_SERVICE_PORT")
	if len(k8sPsbHost) > 0 && len(k8sPsbPort) > 0 {
		gInClusterServiceAddr = fmt.Sprintf("%s:%s", k8sPsbHost, k8sPsbPort)
	}

	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/k8spsb-api/psb/info", handlePSBInfo)
	router.HandleFunc("/k8spsb-api/psb/app-services", deployAppHandler)
	router.HandleFunc("/k8spsb-api/psb/app-services/{space}/{appServiceId}", appServiceInfoHandler)
	router.HandleFunc("/k8spsb-api/psb/app-services/{space}/{appServiceId}/logs", appServiceLogsHandler)
	router.HandleFunc("/k8spsb-api/psb/app-services/{space}/{appServiceId}/logs/data", logsDataHandler)
	router.HandleFunc("/k8spsb-api/psb/spaces", listSpacesHandler)
	router.HandleFunc("/k8spsb-api/state", serviceStateHandler)

	err = http.ListenAndServe(":8080", router)
	if err != nil {
		log.Fatal(err)
	}
	//defer l.Close()
	//l = netutil.LimitListener(l, 10)
	//err = http.Serve(l, nil)
	//	if err != nil {
	//		log.Fatal(err)
	//	}

	//http.ListenAndServe(":8000", nil)
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
