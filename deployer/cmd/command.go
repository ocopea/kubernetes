// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package cmd

import (
	"flag"
	k8sClient "ocopea/kubernetes/client"
	"fmt"
	"os"
	"errors"
	"strings"
)

type DeployerContext struct {
	Client         *k8sClient.Client
	DeploymentType string
	Namespace      string
	ClusterIp      string
}

type DeployerCommand struct {
	FlagSet  *flag.FlagSet
	Name     string
	Executor func(ctx *DeployerContext) error
}

type globalArgsBag struct {
	k8sURL         *string
	k8sNamespace   *string
	deploymentType *string
	localClusterIp *string
	userName       *string
	password       *string
}


func addGlobalFlagsToFlagSet(flagSet *flag.FlagSet) globalArgsBag {
	return globalArgsBag{
		k8sURL : flagSet.String("url", "http://localhost:8080", "K8S remote api url"),
		k8sNamespace : flagSet.String("namespace", "ocopea", "K8S namespace to use"),
		deploymentType : flagSet.String("deployment-type", "local", "Deployment type"),
		localClusterIp : flagSet.String("local-cluster-ip", "", "Local cluster ip - only relevant on local deployments"),
		userName : flagSet.String("user", "", ""),
		password : flagSet.String("password", "", "Password"),

	}

}

func PrintFullUsage(deployerCommands []*DeployerCommand) {
	fmt.Print("Usage: deployer <command> [<args>]\nAvailable commands:\n\n")
	for _, currCommand := range deployerCommands {
		fmt.Printf("%s:\n", currCommand.Name)
		currCommand.FlagSet.PrintDefaults()
	}

	// Create dummy command for global flags
	globalFlags := flag.NewFlagSet("", flag.ExitOnError)
	addGlobalFlagsToFlagSet(globalFlags)
	fmt.Println("\nglobal variables:")
	globalFlags.PrintDefaults()

}

func Exec(deployerCommands []*DeployerCommand) error {

	if len(os.Args) == 1 {
		PrintFullUsage(deployerCommands)
		return nil
	}

	command := os.Args[1]

	var commandToUse *DeployerCommand = nil
	for _, currCommand := range deployerCommands {
		if (currCommand.Name == command) {
			commandToUse = currCommand;
			break
		}
	}

	if (commandToUse == nil) {
		fmt.Printf("%s is not valid command.\n", command)
		PrintFullUsage(deployerCommands)
		return errors.New("Invalid command");
	}

	// Adding global flags to the selected command
	globalFlagsBag := addGlobalFlagsToFlagSet(commandToUse.FlagSet)
	commandToUse.FlagSet.Parse(os.Args[2:])
	err, ctx := initializeDeployerContext(globalFlagsBag)
	if err != nil {
		return err
	}

	// At last! Execute the selected command
	err = commandToUse.Executor(ctx)
	if err != nil {
		return fmt.Errorf("Failed executing command %s - %s\n", commandToUse.Name, err.Error())
	}
	return nil

}

func initializeDeployerContext(globalArgs globalArgsBag) (error, *DeployerContext) {

	fmt.Printf("k8s url: %s\nnamespace: %s\ndeployment: %s\nuser: %s\n",
		*globalArgs.k8sURL, *globalArgs.k8sNamespace, *globalArgs.deploymentType, *globalArgs.userName)


	// input validation
	if (strings.Compare(*globalArgs.deploymentType, "local") == 0 &&
		len(*globalArgs.localClusterIp) == 0) {
		return errors.New("on local deployment, you must provide local-cluster-ip flag, bye.."), nil
	}

	// Building "secure" http client for communicating with the target kubernetes cluster
	client, err := k8sClient.NewClient(*globalArgs.k8sURL, *globalArgs.k8sNamespace, *globalArgs.userName, *globalArgs.password, "")
	if (err != nil) {
		return errors.New("Failed creating connection with kubernetes cluster " + err.Error()), nil
	}

	// Instantiating deployer context struct
	return nil,
		&DeployerContext{
			Namespace: *globalArgs.k8sNamespace,
			Client: client,
			ClusterIp: *globalArgs.localClusterIp,
			DeploymentType: *globalArgs.deploymentType,
		}

}


