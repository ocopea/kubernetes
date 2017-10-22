// Copyright (c) [2017] Dell Inc. or its subsidiaries. All Rights Reserved.
package com.emc.ocopea.scenarios;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.apache.commons.io.IOUtils;
import org.codehaus.plexus.util.InterpolationFilterReader;
import org.jboss.resteasy.client.jaxrs.BasicAuthentication;
import org.junit.Assert;

import javax.ws.rs.client.Client;
import javax.ws.rs.client.Entity;
import javax.ws.rs.client.Invocation;
import javax.ws.rs.client.WebTarget;
import javax.ws.rs.core.MediaType;
import javax.ws.rs.core.Response;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.Reader;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;
import java.util.function.Consumer;

import static net.javacrumbs.jsonunit.JsonAssert.assertJsonEquals;

/**
 * Created by liebea on 6/20/16.
 * Drink responsibly
 */
public abstract class BaseScenario {
    private final String name;
    private String rootUrl;
    private Client client;
    private ScenarioRunner.ScenarioExecutionContext context;

    protected BaseScenario(String name) {
        this.name = name;
    }

    public String getName() {
        return name;
    }

    void init(String rootUrl, Client client, ScenarioRunner.ScenarioExecutionContext context) {
        if (rootUrl.endsWith("/")) {
            rootUrl = rootUrl.substring(0, rootUrl.length() - 1);
        }
        this.rootUrl = rootUrl;
        this.client = client;
        this.context = context;
        initializeScenario();

    }

    protected void initializeScenario() {
    }

    protected ScenarioRunner.ScenarioExecutionContext getContext() {
        return context;
    }

    protected <T> T getFromContext(String name, Class<T> clazz) {
        Object object = context.getLatest(name);
        if (object == null) {
            throw new IllegalArgumentException("missing " + name + " in scenario context");
        }
        if (!clazz.isInstance(object)) {
            throw new IllegalArgumentException(
                    name + " in scenario context is not " + clazz.getName() + " but " + object.getClass().getName());
        }
        return clazz.cast(object);
    }

    protected abstract Map<String, Object> executeScenario();

    private WebTarget getTarget(String path) {
        final WebTarget target = client.target(rootUrl + "/" + path);
        target.register(new BasicAuthentication("shpandrak", "1234"));
        return target;
    }

    public interface RestConverter<T> {
        T convert(Response r);
    }

    public interface RestConsumer<T> {
        void consume(Response r, T value);
    }

    protected <T> T doGet(String path, Class<T> defaultConversionClass) {
        return doGet(path, defaultConversionClass, null);
    }

    protected <T> T doGet(String path, Class<T> defaultConversionClass, RestConsumer<T> consumer) {
        return doGet(path, r -> r.readEntity(defaultConversionClass), consumer);
    }

    protected <T> T doGet(String path, RestConverter<T> converter) {
        return doGet(path, converter, null);
    }

    protected <T> T doGet(String path, RestConverter<T> converter, RestConsumer<T> consumer) {
        Invocation invocation = getTarget(path).request(MediaType.APPLICATION_JSON_TYPE).buildGet();
        System.out.println("Invoking http GET on " + rootUrl + "/" + path);
        Response r = invoke(invocation);
        try {
            if (r.getStatusInfo().getFamily() != Response.Status.Family.SUCCESSFUL) {
                Assert.fail("Http response " + r.getStatusInfo().getReasonPhrase()
                        + (r.hasEntity() ? ": " + r.readEntity(String.class) : ""));
            }

            T value = converter.convert(r);
            if (value instanceof String) {
                printJson(value.toString());
            }

            if (consumer != null) {
                consumer.consume(r, value);
            }
            return value;
        } finally {
            r.close();
        }
    }

    static String convertStreamToString(java.io.InputStream is) {
        java.util.Scanner s = new java.util.Scanner(is, "UTF-8").useDelimiter("\\A");
        return s.hasNext() ? s.next() : "";
    }

    private void printJson(String json) {

        ObjectMapper mapper = new ObjectMapper();
        try {
            Object jsonObj = mapper.readValue(json, Object.class);
            System.out.println(mapper.writerWithDefaultPrettyPrinter().writeValueAsString(jsonObj));
        } catch (IOException e) {
            Assert.fail("Failed parsing json - " + e.getMessage());
            e.printStackTrace();
        }
    }

    protected void postJson(String path, InputStream jsonInputStream, Consumer<Response> consumer) {
        postJson(path, convertStreamToString(jsonInputStream), consumer);
    }

    protected void postJson(String path, String json, Consumer<Response> consumer) {

        System.out.println("Invoking http POST on " + rootUrl + "/" + path + " with body:");
        printJson(json);

        Invocation invocation = getTarget(path).request(MediaType.APPLICATION_JSON_TYPE).buildPost(Entity.json(json));
        Response r = invoke(invocation);
        try {
            consumer.accept(r);
        } finally {
            r.close();
        }
    }

    private Response invoke(Invocation invocation) {
        return invocation.invoke();
    }

    protected String readResourceAsString(String name) {
        return readResourceAsString(name, Collections.emptyMap());
    }

    protected String readResourceAsString(String name, Map<String, String> tokenValues) {
        try (InputStream resourceAsStream = getClass().getClassLoader().getResourceAsStream(name);
                Reader r = new InterpolationFilterReader(
                        new InputStreamReader(resourceAsStream, StandardCharsets.UTF_8), new HashMap<>(tokenValues))) {
            return IOUtils.toString(r);
        } catch (IOException e) {
            throw new IllegalStateException(e);
        }
    }

    protected String getAppTemplateIdFromName(String appTemplateName) {
        final Map[] templates = doGet("hub-web-api/app-template", Map[].class);
        Assert.assertNotNull("App Templates returned null", templates);
        Assert.assertTrue("No app templates found", templates.length > 0);
        final Optional<Map> first = Arrays.stream(templates).filter(map -> map.get("name").equals(appTemplateName))
                .findFirst();
        Assert.assertTrue("App Template by name " + appTemplateName + " not found", first.isPresent());
        final String id = (String) first.get().get("id");
        Assert.assertNotNull(appTemplateName + " didn't return an id", id);
        try {
            UUID.fromString(id);
        } catch (Exception e) {
            e.printStackTrace();
            Assert.fail("Invalid UUID " + id);
        }
        return id;
    }

    protected String waitForAppToDeploy(UUID appInstanceId) {
        int attempt = 0;
        String state = null;

        int maxRetries = 20;
        while (attempt < maxRetries) {
            Map stateMap = doGet("hub-web-api/app-instance/" + appInstanceId + "/state", Map.class,
                    (response, value) -> Assert.assertEquals("Failed getting app instance status",
                            Response.Status.OK.getStatusCode(), response.getStatus()));

            state = stateMap.get("state").toString();

            System.out.println(state);
            switch (state) {
            case "DEPLOYING":
                sleepNoException(3000);
                break;
            case "RUNNING":
                attempt = maxRetries;
                break;
            default:
                Assert.fail("App entered unexpected state: " + state);
            }

            ++attempt;
        }
        return state;
    }

    protected String waitForAppToStop(UUID appInstanceId) {
        int attempt = 0;
        String state = null;

        int maxRetries = 20;
        while (attempt < maxRetries) {
            Map stateMap = doGet("hub-web-api/app-instance/" + appInstanceId + "/state", Map.class,
                    (response, value) -> Assert.assertEquals("Failed getting app instance status",
                            Response.Status.OK.getStatusCode(), response.getStatus()));

            state = stateMap.get("state").toString();

            System.out.println(state);
            switch (state) {
            case "STOPPING":
                sleepNoException(3000);
                break;
            case "STOPPED":
                attempt = maxRetries;
                break;
            default:
                Assert.fail("App entered unexpected state: " + state);
            }

            ++attempt;
        }
        return state;
    }

    protected void sleepNoException(int millis) {
        try {
            Thread.sleep(millis);
        } catch (InterruptedException e) {
            throw new IllegalStateException(e);
        }
    }

    protected void populateServiceConfigurationParams(Map<String, String> tokenValues, UUID siteId,
            UUID appTemplateId) {
        final Map siteSpaces = doGet("hub-web-api/site-topology/" + siteId.toString(), Map.class);

        final List spacesList = (List) siteSpaces.get("spaces");
        if (spacesList.isEmpty()) {
            Assert.fail("site " + siteId + " has no spaces");
        }
        final String space = spacesList.iterator().next().toString();

        final Map supportedConfigurationsOnSite = doGet("hub-web-api/test-dev/site/" + siteId.toString()
                + "/app-template-configuration/" + appTemplateId.toString(), Map.class);

        List appServiceConfigurations = (List) supportedConfigurationsOnSite.get("appServiceConfigurations");
        final String singleServiceName = (String) ((Map) (appServiceConfigurations.get(0))).get("appServiceName");
        Map supportedVersions = (Map) ((Map) (appServiceConfigurations.get(0))).get("supportedVersions");

        if (supportedVersions.isEmpty()) {
            Assert.fail("No supported configuration for app on site");
        }
        final Map.Entry<?, ?> entry = (Map.Entry<?, ?>) supportedVersions.entrySet().iterator().next();
        tokenValues.put("artifactRegistryName", entry.getKey().toString());
        tokenValues.put("svcName", singleServiceName);
        tokenValues.put("svcVersion", ((List) entry.getValue()).iterator().next().toString());
        tokenValues.put("space", space);

        // Expecting a single data service
        @SuppressWarnings("unchecked")
        final List<Map> dataServiceConfigurations = (List<Map>) supportedConfigurationsOnSite
                .get("dataServiceConfigurations");

        if (dataServiceConfigurations.isEmpty()) {
            Assert.fail("No supported data service configuration for app on site");
        }

        final String singeDataServiceName = dataServiceConfigurations.get(0).get("dataServiceName").toString();
        tokenValues.put("dataServiceName", singeDataServiceName);

        parseDataService(tokenValues, supportedConfigurationsOnSite, singeDataServiceName, "dsb");
    }

    private void parseDataService(Map<String, String> tokenValues, Map supportedConfigurationsOnSite,
            String dataServiceName, String prefix) {
        //noinspection unchecked
        ((List<Map>) supportedConfigurationsOnSite.get("dataServiceConfigurations")).stream()
                .filter(m -> dataServiceName.equals(m.get("dataServiceName").toString())).forEach(map -> {
                    final Map dsbPlans = (Map) ((Map) map.get("dsbPlans")).values().iterator().next();
                    tokenValues.put(prefix + "Urn", dsbPlans.get("name").toString());
                    final Map plan = (Map) ((List) dsbPlans.get("plans")).get(0);
                    tokenValues.put(prefix + "Plan", plan.get("id").toString());
                    tokenValues.put(prefix + "Protocol", ((List) plan.get("protocols")).get(0).toString());
                });
    }

    protected void doGetAndValidateJson(String url, String jsonResourcePath, final Map<String, String> tokens) {
        doGet(url, String.class, (r, value) -> assertJsonEquals(readResourceAsString(jsonResourcePath, tokens), value));
    }

}
