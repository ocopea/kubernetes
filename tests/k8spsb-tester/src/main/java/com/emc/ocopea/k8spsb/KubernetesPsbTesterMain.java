// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package com.emc.ocopea.k8spsb;

import com.emc.ocopea.scenarios.ScenarioRunner;
import com.emc.ocopea.scenarios.k8spsb.CreateAppInstanceScenario;
import com.emc.ocopea.scenarios.k8spsb.ValidatePsbInfoScenario;
import com.emc.ocopea.scenarios.mongodsb.CreateDsbInstanceScenario;
import com.emc.ocopea.scenarios.mongodsb.ValidateDsbInfoScenario;
import com.emc.ocopea.scenarios.mongodsb.VerifyServiceInstancesCountScenario;

import java.io.IOException;
import java.net.URL;
import java.sql.SQLException;
import java.util.UUID;

public class KubernetesPsbTesterMain {

    public static void main(String[] args) throws InterruptedException, SQLException, IOException {

        if (args.length < 1) {
            throw new IllegalArgumentException("Missing url command line argument");
        }
        final String rootUrl = args[0];

        // Validating Url validity
        new URL(rootUrl);

        System.out.println("Root Url - " + rootUrl);

        psbInfoTest(rootUrl);
        createAppInstanceTest(rootUrl);

    }

    private static void psbInfoTest(String rootUrl) throws IOException, SQLException {
        new ScenarioRunner(
                rootUrl,
                new ScenarioRunner.TestCase("Validate PSB Info")
                        .then(new ValidatePsbInfoScenario())
        ).run();
    }

    private static void createAppInstanceTest(String rootUrl) throws IOException, SQLException {
        final String appServiceId = UUID.randomUUID().toString();
        new ScenarioRunner(
                rootUrl,
                new ScenarioRunner.TestCase("Create App Instance")
                        .then(new CreateAppInstanceScenario(appServiceId))
        ).run();
    }

}
