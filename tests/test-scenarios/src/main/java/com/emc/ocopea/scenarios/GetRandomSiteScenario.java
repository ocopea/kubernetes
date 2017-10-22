// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package com.emc.ocopea.scenarios;

import org.junit.Assert;

import java.util.HashMap;
import java.util.Map;
import java.util.UUID;

public class GetRandomSiteScenario extends BaseScenario {

    private final String siteIdKeyOut;

    public GetRandomSiteScenario(String siteIdKeyOut) {
        super("Pick Random Site");
        this.siteIdKeyOut = siteIdKeyOut;
    }

    @Override
    protected Map<String, Object> executeScenario() {
        Map<String, Object> c = new HashMap<>();

        final Map[] sites = doGet("hub-web-api/site", Map[].class);
        Assert.assertNotNull("sites null", sites);
        Assert.assertTrue("No sites found", sites.length > 0);
        final String id = (String) sites[0].get("id");
        Assert.assertNotNull(id + " didn't return an id", id);
        try {
            c.put(siteIdKeyOut, UUID.fromString(id));
        } catch (Exception e) {
            e.printStackTrace();
            Assert.fail("Invalid UUID " + id);
        }
        return c;
    }
}
