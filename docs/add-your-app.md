# Adding Your Apps


## App templates 
In the Ocopea terminology, every application topology is described by an "App template" entity.  
In order to create a new app template users should use the "create-app-template" command of the hub-webapp 
Ocopea service. See [invoking Commands](invoking-ocopea-commands.md)

## Create App Template Command

The "create-app-template" command expects the user to provide full description of the application topology as described
in the 
[hub-web swagger API](https://github.com/ocopea/orcs/blob/master/hub/webapp/web-api/src/main/resources/swagger.yaml).

We'll use the [lets-chat](https://hub.docker.com/r/sdelements/lets-chat/) app template to understand the building block

```
{
  "name":"lets chat",
  "version":"1.0",
  "description":"Lets chat cool app",
  "entryPointServiceName":"lets-chat",
  "appServiceTemplates":[
    {
      "appServiceName":"lets-chat",
      "psbType":"k8s",
      "imageName":"sdelements/lets-chat",
      "imageType":"nodejs",
      "imageVersion":"latest",
      "psbSettings":{},
      "environmentVariables":{
        "LCB_DATABASE_URI":"mongodb://${chat-db.server}/letschat"
      },
      "dependencies":[
        {
          "type":"DATABASE",
          "name":"chat-db",
          "description":"Chats database",
          "protocols":[
            {
              "protocolName":"mongodb",
              "version":null,
              "conditions":null
            }
          ]
        }
      ],
      "exposedPorts":[
        8080
      ],
      "httpPort":8080,
      "entryPointUrl":"login"
    }
  ]
}
```

General template fields are pretty straight forward:

<br/>

|property|description|
|---|---|
|name|template name|
|version|template version|
|description|template additional description|
|entryPointServiceName|name of the service from the "appServiceTemplates" section that will be used as main entry point for users. it's URL is going to be exposed as the app URL |
|appServiceTemplates| Array of all services that are part of this app template|

<br/>

For each microservice the app includes, add an entry under appServiceTemplates, the app service template format:

<br/>

|property|description|
|---|---|
|appServiceName|display name of the service, will be presented in the ocopea app topology|
|psbType|the orchestrator the app is designated to use - for Kubernetes, use k8s|
|imageName|The full docker image name for running the app|
|imageType|nodejs|Base docker image type/ technology - used for displaying the app type icon on the app topology view|
|imageVersion|It is recommended to use a specific docker tag for the image, otherwise use "latest"|
|environmentVariables|Environment variables expected by the container. it is possible to inject dependency bindings into the variable. In our case we injected the MongoDB server address as part of the environment variable value  
|dependencies|Array of infrastructure service dependencies of the app service|
|exposedPorts| Array of ports the app service expects Kubernetes to expose externally|
|httpPort| In case of multiple ports exposed, which port should be used for http access (if applicable). Kubernetes will expose this port externally as port 80 if possible|
|entryPointUrl|Relative path for entry point url for the service|

<br/>

For each dependency of the app service, add an entry under "dependencies". the dependencies format:

<br/>

|property|description|
|---|---|
|type|type of the dependency, used for display only DATABASE,OBJECTSTORE,MESSAGING,CACHE,SCHEDULER,LOGGING,VOLUME,OTHER
|name|dependency display name|
|description|Additional description for the dependency and what it is used for|
|protocols|array supported protocols. This is an array since some apps support alternative protocols (e.g. can use either postgres or mysql according to what is available)|

<br/>

The dependency protocol format:

<br/>

|property|description|
|---|---|
|protocolName|name of the protocol - will be matched with DSB supported protocols. e.g. mongodsb,postgres,mysql,s3,docker-volume...|
|version| optional - use the version field if the service required a minimal version of the protocol
|conditions| optional - the conditions can be used to restrict protocol attributes for specific protocol requirements 

<br/>

## Generic App Templates

Application templates are built to be as generic as possible, so that the apps could run on as many different clusters as 
possible. For example:
- External dependencies are described as dependencies of protocols and not specific services so that dependency could 
be satisfied by different vendor services. If the service requires S3 in order to store documents, we'll depend on 
the S3 protocol and not the AWS S3 service - this way the application can run on other environments that support the 
popular S3 protocol (e.g. minio)
- Templates include only the docker image names and not the docker registry they should be taken from. Each cluster 
can be configured with different docker registries private or public

## Adding App Template Icon

In order to make Ocopea display the application template icon in the Ocopea UI, you'll need to upload the application 
template icon via the ocopea API. To do that you'll need the appTemplateId returned by the create-app-template command.
You can find the appTemplateId by executing a GET command on the Ocopea API on URL hub-web-api/app-template.
To upload the icon, POST the image body to URL hub-web-api/images/app-template/{appTemplateId}

## Automatic discovery

While we would eventually like to automatically discover your application topology, 
at this stage adding support for a new application is a manual step and supported via API only (not UI).
The main challenge in automatic discovery stems from the fact that both Kubernetes deployment descriptors is focused 
on deployment instructions and lacks metadata required by ocopea in order to create application copies.


