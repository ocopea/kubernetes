package com.emc.ocopea.scenarios;

import org.junit.Assert;

import javax.ws.rs.core.Response;
import java.util.HashMap;
import java.util.Map;
import java.util.UUID;

/**
 * Created by liebea on 6/20/16.
 * Drink responsibly
 */
public class DeploySavedImageScenario extends BaseScenario {

    private final String savedImageIdKeyIn;
    private UUID savedImageId;
    private UUID siteId;
    private final String siteIdKeyIn;
    private final String appInstanceName;
    private String appInstanceIdKeyOut;

    public DeploySavedImageScenario(
            String appInstanceName,
            String savedImageIdKeyIn,
            String siteIdKeyIn,
            String appInstanceIdKeyOut) {
        super("Deploy Saved Image");
        this.savedImageIdKeyIn = savedImageIdKeyIn;
        this.appInstanceName = appInstanceName;
        this.appInstanceIdKeyOut = appInstanceIdKeyOut;
        this.siteIdKeyIn = siteIdKeyIn;
    }

    @Override
    protected void initializeScenario() {
        this.savedImageId = getFromContext(savedImageIdKeyIn, UUID.class);
        this.siteId = getFromContext(siteIdKeyIn, UUID.class);
    }

    @Override
    protected Map<String, Object> executeScenario() {

        // Deploy the hackathon app template
        final Map<String, String> tokenValues = new HashMap<>();
        tokenValues.put("savedImageId", savedImageId.toString());
        tokenValues.put("appInstanceName", appInstanceName);
        tokenValues.put("siteId", siteId.toString());

        final UUID appTemplateId =
                UUID.fromString(
                        doGet(
                                "hub-web-api/test-dev/saved-app-images/" + savedImageId.toString(), Map.class)
                                .get("appTemplateId").toString());

        populateServiceConfigurationParams(tokenValues, siteId, appTemplateId);

        final Map<String, Object> contextToReturn = new HashMap<>();
        postJson(
                "hub-web-api/commands/deploy-saved-image",
                readResourceAsString("simple-template/deploy-saved-image-command-args.json", tokenValues),
                (r) -> {
                    // Testing that the command succeeded
                    Assert.assertEquals(
                            "Failed executing deploy-saved-image command",
                            Response.Status.CREATED.getStatusCode(),
                            r.getStatus());

                    final UUID appInstanceId = r.readEntity(UUID.class);
                    Assert.assertNotNull(appInstanceId);
                    contextToReturn.put(appInstanceIdKeyOut, appInstanceId);

                    String state = waitForAppToDeploy(appInstanceId);
                    Assert.assertEquals("RUNNING", state);
                });
        return contextToReturn;
    }
}
