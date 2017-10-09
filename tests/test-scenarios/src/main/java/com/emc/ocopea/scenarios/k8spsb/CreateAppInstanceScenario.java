package com.emc.ocopea.scenarios.k8spsb;

import com.emc.ocopea.scenarios.BaseScenario;
import org.junit.Assert;

import javax.ws.rs.core.Response;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

public class CreateAppInstanceScenario extends BaseScenario {
    private final String appServiceId;

    public CreateAppInstanceScenario(String appServiceId) {
        super("Create PSB App Instance");
        this.appServiceId = appServiceId;
    }

    @Override
    protected Map<String, Object> executeScenario() {
        postJson(
                "psb/app-services",
                readResourceAsString(
                        "k8spsb/createAppService.json",
                        new HashMap<String, String>(){{
                            put("appServiceId", CreateAppInstanceScenario.this.appServiceId);
                        }}),
                response -> Assert.assertEquals(Response.Status.CREATED.getStatusCode(), response.getStatus())
        );
        return Collections.emptyMap();
    }
}
