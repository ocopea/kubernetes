# Invoking Ocopea Commands

In order to invoke commands via the Ocopea API you'll need to access the ocopea api server called hub-web.
The hub-web component is running as part of the ocopea orcs docker image and can be accessed externally to the cluster.

## Finding the hub-web url

Use kubectl client to find the orcs service address
```
kubectl describe svc orcs --namespace=ocopea
```

In case you are using local deployment, the port will be the exposed service "NodePort", otherwise it will be port 80.
The url of the hub-component will be 
```
http://{ orcs service address } [: orcs service port]/hub-web-api
```


## Invoking the command

to invoke Ocopea API commands post the command body to
```
http://{orcs service url}/hub-web-api/commands/{command name}
``` 

Ocopea API server uses basic authentication, so each command should be posted with your Ocopea username and password

For more information about the available commands see the 
[hub-web swagger descriptor](https://github.com/ocopea/orcs/blob/master/hub/webapp/web-api/src/main/resources/swagger.yaml)
