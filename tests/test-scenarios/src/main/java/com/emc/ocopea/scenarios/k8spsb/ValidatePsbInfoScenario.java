package com.emc.ocopea.scenarios.k8spsb;

import com.emc.ocopea.scenarios.BaseScenario;

import java.util.Collections;
import java.util.Map;

public class ValidatePsbInfoScenario extends BaseScenario {

    public ValidatePsbInfoScenario() {
        super("Validate PSB Info");
    }

    @Override
    protected Map<String, Object> executeScenario() {

        doGetAndValidateJson(
                "psb/info",
                "k8spsb/psbInfo.json",
                Collections.emptyMap()
        );

        return Collections.emptyMap();
    }
}
