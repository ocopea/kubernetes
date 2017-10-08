package com.emc.ocopea.scenarios.mongodsb;

import com.emc.ocopea.scenarios.BaseScenario;

import java.util.Collections;
import java.util.Map;

/**
 * Created by liebea on 6/20/16.
 * Drink responsibly
 */
public class ValidateDsbInfoScenario extends BaseScenario {

    public ValidateDsbInfoScenario() {
        super("Validate DSB Info");
    }

    @Override
    protected Map<String, Object> executeScenario() {

        doGetAndValidateJson(
                "",
                "mongodsb/dsbInfo.json",
                Collections.emptyMap()
        );

        return Collections.emptyMap();
    }
}
