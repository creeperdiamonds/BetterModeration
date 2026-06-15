package xyz.creeperdiamonds.bettermoderation.fabric.sync;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import xyz.creeperdiamonds.bettermoderation.core.domain.Punishment;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.CompletableFuture;

/**
 * Async HTTP client for communicating with the BetterModeration backend API
 * from within the Fabric mod.
 */
public class BackendClient {

    private final String baseUrl;
    private final String serverId;
    private final String apiKey;
    private final HttpClient httpClient;
    private final ObjectMapper mapper;

    public BackendClient(String baseUrl, String serverId, String apiKey) {
        this.baseUrl = baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;
        this.serverId = serverId;
        this.apiKey = apiKey;
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(5))
                .build();
        this.mapper = new ObjectMapper();
    }

    /**
     * Fetches all active punishments for a player identified by UUID.
     * If ip is non-null it is forwarded so the backend can track it and enforce IP bans.
     */
    public CompletableFuture<List<Punishment>> getActivePunishments(String minecraftUuid, String ip) {
        String path = "/v1/minecraft/" + minecraftUuid + "/punishments";
        if (ip != null && !ip.isBlank()) {
            path += "?ip=" + java.net.URLEncoder.encode(ip, java.nio.charset.StandardCharsets.UTF_8);
        }
        HttpRequest request = baseRequest(path)
                .GET()
                .build();

        return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString())
                .thenApply(response -> {
                    if (response.statusCode() != 200) {
                        return List.of();
                    }
                    try {
                        return mapper.readValue(response.body(), new TypeReference<List<Punishment>>() {});
                    } catch (Exception e) {
                        return List.<Punishment>of();
                    }
                });
    }

    /**
     * Notifies the backend that this server has started or stopped.
     *
     * @param online true if the server just started, false if it is stopping
     */
    public CompletableFuture<Void> notifyServerStatus(boolean online) {
        try {
            String body = mapper.writeValueAsString(new java.util.HashMap<>() {{
                put("serverId", serverId);
                put("online", online);
            }});

            HttpRequest request = baseRequest("/v1/servers/status")
                    .PUT(HttpRequest.BodyPublishers.ofString(body))
                    .header("Content-Type", "application/json")
                    .build();

            return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString())
                    .thenApply(r -> null);
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    /**
     * Returns true if any alt account of the given Minecraft UUID has an active ban.
     */
    public CompletableFuture<Boolean> hasAltWithActiveBan(String minecraftUuid) {
        HttpRequest request = baseRequest("/v1/minecraft/" + minecraftUuid + "/alts")
                .GET()
                .build();

        return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString())
                .thenApply(response -> {
                    if (response.statusCode() != 200) return false;
                    try {
                        com.fasterxml.jackson.databind.JsonNode arr = mapper.readTree(response.body());
                        for (com.fasterxml.jackson.databind.JsonNode alt : arr) {
                            if (alt.path("has_active_ban").asBoolean(false)) return true;
                        }
                        return false;
                    } catch (Exception e) {
                        return false;
                    }
                });
    }

    private HttpRequest.Builder baseRequest(String path) {
        return HttpRequest.newBuilder()
                .uri(URI.create(baseUrl + path))
                .timeout(Duration.ofSeconds(10))
                .header("X-Server-Id", serverId)
                .header("Authorization", "Bearer " + apiKey);
    }
}
