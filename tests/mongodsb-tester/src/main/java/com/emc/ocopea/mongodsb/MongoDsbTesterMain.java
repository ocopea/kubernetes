// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package com.emc.ocopea.mongodsb;

import com.emc.ocopea.scenarios.ScenarioRunner;
import com.emc.ocopea.scenarios.mongodsb.CreateDsbInstanceScenario;
import com.emc.ocopea.scenarios.mongodsb.ValidateDsbInfoScenario;
import com.emc.ocopea.scenarios.mongodsb.VerifyServiceInstancesCountScenario;

import java.io.IOException;
import java.net.URL;
import java.sql.SQLException;
import java.util.UUID;

public class MongoDsbTesterMain {

    public static void main(String[] args) throws InterruptedException, SQLException, IOException {

        if (args.length < 1) {
            throw new IllegalArgumentException("Missing url command line argument");
        }
        final String rootUrl = args[0];

        // Validating Url validity
        new URL(rootUrl);

        System.out.println("Root Url - " + rootUrl);

        createDsbInstanceTest(rootUrl);
        dsbInfoTest(rootUrl);

    }

    private static void createDsbInstanceTest(String rootUrl) {
        final String mongoServiceId = UUID.randomUUID().toString();

        new ScenarioRunner(
                rootUrl,
                new ScenarioRunner.TestCase("Create DSB Instance")
                        .then(new VerifyServiceInstancesCountScenario(0))
                        .then(new CreateDsbInstanceScenario(mongoServiceId, "standard"))
                        .then(new VerifyServiceInstancesCountScenario(1)))
                .run();
    }

    private static void dsbInfoTest(String rootUrl) throws IOException, SQLException {
        new ScenarioRunner(
                rootUrl,
                new ScenarioRunner.TestCase("Validate DSB Info")
                        .then(new ValidateDsbInfoScenario())
        ).run();
    }

}
