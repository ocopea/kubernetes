package com.emc.ocopea.scenarios;

import org.jboss.resteasy.plugins.providers.jackson.ResteasyJackson2Provider;
import org.junit.Assert;

import javax.ws.rs.client.Client;
import javax.ws.rs.client.ClientBuilder;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedList;
import java.util.List;
import java.util.Map;

/**
 * Created by liebea on 8/11/16.
 * Drink responsibly
 */
public class ScenarioRunner {
    private final String rootUrl;
    private final TestCase execution;

    public ScenarioRunner(String rootUrl, TestCase execution) {
        this.rootUrl = rootUrl;
        this.execution = execution;
    }

    public class ScenarioExecutionContext {
        // list in reverse order of insertion
        private final Map<String, List<Object>> executionResults = new HashMap<>();

        /**
         * returns results from context by key, ordered from last inserted to first.
         */
        public <T> List<T> getResults(String key) {
            final List<Object> objects = executionResults.get(key);
            if (objects.isEmpty()) {
                Assert.fail("Scenario expected to have " + key + " in execution context");
            }
            //noinspection unchecked
            return (List<T>) objects;

        }

        public <T> T getLatest(String key) {
            final List<Object> objects = getResults(key);
            //noinspection unchecked
            return (T) objects.get(0);
        }

        void addResult(Map<String, Object> result) {
            result.forEach((s, o) -> {
                List<Object> objects = executionResults.get(s);
                if (objects == null) {
                    objects = new LinkedList<>();
                    executionResults.put(s, objects);
                }
                objects.add(0, o);
            });
        }
    }

    public static class TestCase {
        private final String testCaseName;
        private final List<BaseScenario> scenarios = new ArrayList<>();

        public TestCase(String testCaseName) {
            this.testCaseName = testCaseName;
        }

        public String getTestCaseName() {
            return testCaseName;
        }

        public TestCase then(BaseScenario scenario) {
            scenarios.add(scenario);
            return this;
        }
    }

    public void run() {

        System.out.println("Executing Test Case \"" + execution.getTestCaseName() + "\"");
        final ScenarioExecutionContext context = new ScenarioExecutionContext();
        Client client = ClientBuilder.newBuilder()
                .register(new ResteasyJackson2Provider())
                .build();

        try {
            execution.scenarios.forEach(
                    scenario ->
                            executeScenario(context, client, scenario));
            System.out.println("Test Case \"" + execution.getTestCaseName() + "\" PASSED!");
        }catch (Throwable th) {
            System.out.println("Test Case \"" + execution.getTestCaseName() + "\" FAILED!");
            throw th;
        }
        finally {
            client.close();
        }
    }

    private void executeScenario(
            ScenarioExecutionContext context,
            Client client,
            BaseScenario scenario) {

        try {
            System.out.println("Initializing Test Scenario \"" + scenario.getName() + "\"");
            scenario.init(rootUrl, client, context);
            System.out.println("Executing Test Scenario \"" + scenario.getName() + "\"");
            final Map<String, Object> result = scenario.executeScenario();
            context.addResult(result);
            System.out.println("Test Scenario \"" + scenario.getName() + "\" PASSED!");
        } catch (Throwable th) {
            System.out.println("Test Scenario \"" + scenario.getName() + "\" FAILED!");
            throw th;
        }
    }

}
