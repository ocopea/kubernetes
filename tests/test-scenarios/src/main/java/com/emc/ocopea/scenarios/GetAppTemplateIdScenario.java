// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package com.emc.ocopea.scenarios;

import java.util.Collections;
import java.util.Map;
import java.util.UUID;

/**
 * Created by liebea on 6/20/16.
 * Drink responsibly
 */
public class GetAppTemplateIdScenario extends BaseScenario {

    private final String appTemplateName;
    private String appTemplateIdKeyOut;

    public GetAppTemplateIdScenario(String appTemplateName, String appTemplateIdKeyOut) {
        super("Get appTemplateId");
        this.appTemplateName = appTemplateName;
        this.appTemplateIdKeyOut = appTemplateIdKeyOut;
    }

    @Override
    protected Map<String, Object> executeScenario() {
        return Collections.singletonMap(
                appTemplateIdKeyOut,
                UUID.fromString(getAppTemplateIdFromName(appTemplateName)));
    }
}
