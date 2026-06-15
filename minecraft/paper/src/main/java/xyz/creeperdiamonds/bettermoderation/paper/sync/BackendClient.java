package xyz.creeperdiamonds.bettermoderation.paper.sync;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import xyz.creeperdiamonds.bettermoderation.core.domain.ConnectResponse;
import xyz.creeperdiamonds.bettermoderation.core.domain.Punishment;
import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.CompletableFuture;

/**
 * Async HTTP client for communicating with the BetterModeration backend API.
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
     * Fetches all active punishments for a given Minecraft UUID.
     * If ip is non-null it is sent so the backend can track it and enforce IP bans.
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
     * Issues a punishment against a player identified by their Minecraft UUID.
     */
    public CompletableFuture<Punishment> issuePunishment(
            String minecraftUuid,
            PunishmentType type,
            String reason,
            String issuedBy,
            Long expiresAt) {
        try {
            String body = mapper.writeValueAsString(new java.util.HashMap<>() {{
                put("minecraftUuid", minecraftUuid);
                put("type", type.name());
                put("reason", reason);
                put("issuedBy", issuedBy);
                put("expiresAt", expiresAt);
                put("serverId", serverId);
            }});

            HttpRequest request = baseRequest("/v1/punishments")
                    .POST(HttpRequest.BodyPublishers.ofString(body))
                    .header("Content-Type", "application/json")
                    .build();

            return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString())
                    .thenApply(response -> {
                        try {
                            return mapper.readValue(response.body(), Punishment.class);
                        } catch (Exception e) {
                            return null;
                        }
                    });
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    /**
     * Requests a one-time link code for the given Minecraft UUID.
     */
    public CompletableFuture<String> generateLinkCode(String minecraftUuid) {
        try {
            String body = mapper.writeValueAsString(new java.util.HashMap<>() {{
                put("minecraftUuid", minecraftUuid);
            }});

            HttpRequest request = baseRequest("/v1/link/generate")
                    .POST(HttpRequest.BodyPublishers.ofString(body))
                    .header("Content-Type", "application/json")
                    .build();

            return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString())
                    .thenApply(response -> {
                        try {
                            return mapper.readTree(response.body()).path("code").asText();
                        } catch (Exception e) {
                            return null;
                        }
                    });
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    /**
     * Submits a player report to the backend.
     */
    public CompletableFuture<Void> reportPlayer(String reporterUuid, String targetUuid, String reason) {
        try {
            String body = mapper.writeValueAsString(new java.util.HashMap<>() {{
                put("reporterUuid", reporterUuid);
                put("targetUuid", targetUuid);
                put("reason", reason);
                put("serverId", serverId);
            }});

            HttpRequest request = baseRequest("/v1/reports")
                    .POST(HttpRequest.BodyPublishers.ofString(body))
                    .header("Content-Type", "application/json")
                    .build();

            return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString())
                    .thenApply(r -> null);
        } catch (Exception e) {
            return CompletableFuture.failedFuture(e);
        }
    }

    /**
     * Notifies the backend that a player disconnected (session tracking).
     */
    public CompletableFuture<Void> notifyDisconnect(String minecraftUuid) {
        try {
            String body = mapper.writeValueAsString(new java.util.HashMap<>() {{
                put("minecraftUuid", minecraftUuid);
                put("serverId", serverId);
            }});

            HttpRequest request = baseRequest("/v1/sessions/disconnect")
                    .POST(HttpRequest.BodyPublishers.ofString(body))
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
     * Used on player join to catch ban evasion.
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

    /**
     * Unified join endpoint: tracks the player, scores evasion signals, and returns
     * the recommended action (ALLOW / FLAG / DENY) plus a kick message if denied.
     * Returns null on non-200 or network error (caller should fail-open).
     */
    public CompletableFuture<ConnectResponse> sessionConnect(
            String uuid, String username, String ip, boolean offlineMode) {
        try {
            String body = mapper.writeValueAsString(new java.util.HashMap<>() {{
                put("uuid", uuid);
                put("username", username);
                put("ip", ip != null ? ip : "");
                put("offline_mode", offlineMode);
            }});

            HttpRequest request = baseRequest("/v1/sessions/connect")
                    .POST(HttpRequest.BodyPublishers.ofString(body))
                    .header("Content-Type", "application/json")
                    .build();

            return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString())
                    .thenApply(response -> {
                        if (response.statusCode() != 200) return null;
                        try {
                            return mapper.readValue(response.body(), ConnectResponse.class);
                        } catch (Exception e) {
                            return null;
                        }
                    })
                    .exceptionally(e -> null);
        } catch (Exception e) {
            return CompletableFuture.completedFuture(null);
        }
    }

    /** Shuts down the underlying HTTP client executor (no-op for the default client). */
    public void shutdown() {
        // HttpClient with default executor needs no explicit shutdown.
    }

    private HttpRequest.Builder baseRequest(String path) {
        return HttpRequest.newBuilder()
                .uri(URI.create(baseUrl + path))
                .timeout(Duration.ofSeconds(10))
                .header("X-Server-Id", serverId)
                .header("Authorization", "Bearer " + apiKey);
    }
}
