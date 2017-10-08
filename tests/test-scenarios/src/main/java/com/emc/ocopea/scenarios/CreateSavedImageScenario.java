package com.emc.ocopea.scenarios;

import org.junit.Assert;

import javax.ws.rs.core.Response;
import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

/**
 * Created by liebea on 6/20/16.
 * Drink responsibly
 */
public class CreateSavedImageScenario extends BaseScenario {

    private final String appInstanceIdKeyIn;
    private final String savedImageIdKeyOut;
    private final String imageName;
    private final Set<String> imageTags;
    private final String imageComment;
    private UUID appInstanceId;

    public CreateSavedImageScenario(
            String imageName,
            Set<String> imageTags,
            String imageComment,
            String appInstanceIdKeyIn,
            String savedImageIdKeyOut) {
        super("Create Saved Image");
        this.appInstanceIdKeyIn = appInstanceIdKeyIn;
        this.imageName = imageName;
        this.imageTags = imageTags;
        this.imageComment = imageComment;
        this.savedImageIdKeyOut = savedImageIdKeyOut;
    }

    @Override
    protected void initializeScenario() {
        this.appInstanceId = getFromContext(appInstanceIdKeyIn, UUID.class);
    }

    @Override
    protected Map<String, Object> executeScenario() {
        // Deploy the hackathon app template
        final Map<String, String> tokenValues = new HashMap<>();
        tokenValues.put("image.name", imageName);
        tokenValues.put("image.comment", imageComment);
        tokenValues.put("image.tags", parseTags(imageTags));
        tokenValues.put("image.appInstanceId", appInstanceId.toString());

        final Map<String, Object> contextToReturn = new HashMap<>();
        postJson(
                "hub-web-api/commands/create-saved-image",
                readResourceAsString("saved-image/create-image-command-args" + ".json", tokenValues),
                (r) -> {
                    // Testing that the command succeeded
                    Assert.assertEquals(
                            "Failed executing create-saved-image command",
                            Response.Status.CREATED.getStatusCode(),
                            r.getStatus());

                    final UUID savedImageId = r.readEntity(UUID.class);
                    Assert.assertNotNull(savedImageId);
                    contextToReturn.put(savedImageIdKeyOut, savedImageId);
                });
        return contextToReturn;
    }

    private String parseTags(Set<String> tags) {
        Iterator<String> it = tags.iterator();
        if (!it.hasNext()) {
            return "[]";
        }

        StringBuilder sb = new StringBuilder();
        sb.append('[');
        for (;;) {
            String e = it.next();
            sb.append("\"").append(e).append("\"");
            if (!it.hasNext()) {
                return sb.append(']').toString();
            }
            sb.append(',').append(' ');
        }

    }
}
