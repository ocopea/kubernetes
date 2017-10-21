// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package com.emc.ocopea.k8s;

import com.emc.ocopea.scenarios.CreateSavedImageScenario;
import com.emc.ocopea.scenarios.DeploySavedImageScenario;
import com.emc.ocopea.scenarios.DeployTestDevApplicationScenario;
import com.emc.ocopea.scenarios.GetAppTemplateIdScenario;
import com.emc.ocopea.scenarios.GetRandomSiteScenario;
import com.emc.ocopea.scenarios.ScenarioRunner;
import com.emc.ocopea.scenarios.WaitForSaveImageToCreate;

import java.net.MalformedURLException;
import java.net.URL;
import java.util.Arrays;
import java.util.HashSet;

public class KubernetesDeployerTesterMain {

    public static void main(String[] args) throws MalformedURLException {

        if (args.length < 1) {
            throw new IllegalArgumentException("Missing url command line argument");
        }
        final String rootUrl = args[0];

        // Validating Url validity
        new URL(rootUrl);

        System.out.println("Root Url - " + rootUrl);

        createAndDeploySavedImageTest(rootUrl);

    }

    private static void createAndDeploySavedImageTest(final String rootUrl) {
        new ScenarioRunner(
                rootUrl,
                new ScenarioRunner.TestCase("Create and Deploy Saved Image")
                        .then(new GetAppTemplateIdScenario("lets chat", "appTemplate.lets-chat.id"))
                        .then(new GetRandomSiteScenario("site.id"))
                        .then(new DeployTestDevApplicationScenario(
                                "chat-td1",
                                "appTemplate.lets-chat.id",
                                "site.id",
                                "appInstance.chat-td1.id"))
                        .then(new CreateSavedImageScenario(
                                "myFirstImage",
                                new HashSet<>(Arrays.asList("test/dev", "customer", "perf")),
                                "wooo image",
                                "appInstance.chat-td1.id",
                                "savedImage.myFirstImage.id"))
                        .then(new WaitForSaveImageToCreate("savedImage.myFirstImage.id", 60))
                        .then(new DeploySavedImageScenario(
                                "from-saved-image",
                                "savedImage.myFirstImage.id",
                                "site.id",
                                "appInstance.from-saved-image.id"))
        ).run();
    }

}
