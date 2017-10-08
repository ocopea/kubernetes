package com.emc.ocopea.scenarios;

import org.junit.Assert;

import javax.ws.rs.core.Response;
import java.util.HashMap;
import java.util.Map;
import java.util.UUID;

public class WaitForSaveImageToCreate extends BaseScenario {

    private final String savedImageIdKeyIn;
    private UUID savedImageId;
    private final long timeOutInSeconds;

    public WaitForSaveImageToCreate(String savedImageIdKeyIn, long timeOutInSeconds) {
        super("Wait for saved image to create");
        this.savedImageIdKeyIn = savedImageIdKeyIn;
        this.timeOutInSeconds = timeOutInSeconds;
    }

    @Override
    protected void initializeScenario() {
        this.savedImageId = getFromContext(savedImageIdKeyIn, UUID.class);
    }

    @Override
    protected Map<String, Object> executeScenario() {
        Map<String, Object> contextToReturn = new HashMap<>();

        long started = System.currentTimeMillis();

        final boolean []created = {false};
        while (!created[0]) {
            doGet("hub-web-api/test-dev/saved-app-images/" + savedImageId.toString(), Map.class, (r, value) -> {

                Assert.assertEquals("Failed getting copy metadata", Response.Status.OK.getStatusCode(), r.getStatus());
                created[0] = value.get("state").equals("created");
            });

            if (!created[0]) {
                if (started + (timeOutInSeconds * 1000) < System.currentTimeMillis()) {
                    Assert.fail("Took too long for copy to create, bye now");
                }
                sleepNoException(1000);
            }
        }
        return contextToReturn;
    }

}
