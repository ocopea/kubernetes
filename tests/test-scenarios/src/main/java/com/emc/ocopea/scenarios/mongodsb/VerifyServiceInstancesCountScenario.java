package com.emc.ocopea.scenarios.mongodsb;

import com.emc.ocopea.scenarios.BaseScenario;
import org.junit.Assert;

import java.util.Collections;
import java.util.List;
import java.util.Map;

/**
 * Created by liebea on 10/6/17.
 * Drink responsibly
 */
public class VerifyServiceInstancesCountScenario extends BaseScenario {

    private final int expectedNumberOfServiceInstances;

    public VerifyServiceInstancesCountScenario(int expectedNumberOfServiceInstances) {
        super("Verify Service Instances Count");
        this.expectedNumberOfServiceInstances = expectedNumberOfServiceInstances;
    }

    @Override
    protected Map<String, Object> executeScenario() {
        final List instances = doGet("service_instances", List.class);
        Assert.assertEquals(expectedNumberOfServiceInstances, instances.size());
        return Collections.emptyMap();
    }
}
