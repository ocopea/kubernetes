package com.emc.ocopea.scenarios.mongodsb;

import com.emc.ocopea.scenarios.BaseScenario;
import org.junit.Assert;

import javax.ws.rs.core.Response;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

/**
 * Created by liebea on 10/6/17.
 * Drink responsibly
 */
public class CreateDsbInstanceScenario extends BaseScenario {

    private final String serviceInstanceId;
    private final String servicePlan;

    public CreateDsbInstanceScenario(String serviceInstanceId, String servicePlan) {
        super("Create DSB Instance");
        this.serviceInstanceId = serviceInstanceId;
        this.servicePlan = servicePlan;
    }

    @Override
    protected Map<String, Object> executeScenario() {
        postJson(
                "service_instances",
                readResourceAsString(
                        "mongodsb/createDsbInstance.json",
                        new HashMap<String, String>(){{
                            put("serviceInstanceId", CreateDsbInstanceScenario.this.serviceInstanceId);
                            put("dsbPlan", CreateDsbInstanceScenario.this.servicePlan);
                        }}),
                response -> Assert.assertEquals(response.getStatus(), Response.Status.OK.getStatusCode())
        );
        return Collections.emptyMap();
    }
}
