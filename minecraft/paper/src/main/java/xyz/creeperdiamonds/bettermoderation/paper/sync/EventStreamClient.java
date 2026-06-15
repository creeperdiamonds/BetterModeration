package xyz.creeperdiamonds.bettermoderation.paper.sync;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.bukkit.Bukkit;
import org.bukkit.entity.Player;
import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.UUID;
import java.util.logging.Level;

/**
 * Connects to the backend SSE stream and handles real-time punishment events.
 * Runs on a daemon thread — the server does not need a Redis connection.
 */
public class EventStreamClient implements Runnable {

    private static final long RECONNECT_DELAY_MS = 5_000;

    private final BetterModerationPlugin plugin;
    private final String streamUrl;
    private final String serverId;
    private final String apiKey;
    private final ObjectMapper mapper = new ObjectMapper();

    private volatile boolean running = true;

    public EventStreamClient(BetterModerationPlugin plugin, String baseUrl, String serverId, String apiKey) {
        this.plugin = plugin;
        this.streamUrl = (baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl)
                + "/v1/events/stream";
        this.serverId = serverId;
        this.apiKey = apiKey;
    }

    public void stop() {
        running = false;
    }

    @Override
    public void run() {
        HttpClient client = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(10))
                .build();

        while (running) {
            try {
                HttpRequest request = HttpRequest.newBuilder()
                        .uri(URI.create(streamUrl))
                        .header("Authorization", "Bearer " + apiKey)
                        .header("X-Server-Id", serverId)
                        .header("Accept", "text/event-stream")
                        .timeout(Duration.ofMinutes(5))
                        .GET()
                        .build();

                HttpResponse<java.io.InputStream> response = client.send(
                        request, HttpResponse.BodyHandlers.ofInputStream());

                if (response.statusCode() != 200) {
                    plugin.getLogger().warning("[EventStream] Backend returned " + response.statusCode() + ", retrying...");
                    sleep();
                    continue;
                }

                plugin.getLogger().info("[EventStream] Connected to backend event stream.");

                try (BufferedReader reader = new BufferedReader(new InputStreamReader(response.body()))) {
                    String eventType = null;
                    String line;
                    while (running && (line = reader.readLine()) != null) {
                        if (line.startsWith("event:")) {
                            eventType = line.substring(6).trim();
                        } else if (line.startsWith("data:")) {
                            String data = line.substring(5).trim();
                            if (eventType != null) {
                                handleEvent(eventType, data);
                            }
                            eventType = null;
                        }
                        // blank lines separate events — reset handled above
                    }
                }

                plugin.getLogger().info("[EventStream] Connection closed, reconnecting...");

            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return;
            } catch (Exception e) {
                plugin.getLogger().log(Level.WARNING, "[EventStream] Error: " + e.getMessage() + ", reconnecting...");
            }
            sleep();
        }
    }

    private void handleEvent(String eventType, String data) {
        try {
            JsonNode event = mapper.readTree(data);
            String minecraftUuid = event.path("minecraft_uuid").asText(null);
            String type = event.path("type").asText();
            String reason = event.path("reason").asText("You have been " + type.toLowerCase() + ".");

            if (minecraftUuid == null || minecraftUuid.isBlank()) {
                return; // player not linked to a Minecraft account
            }

            UUID uuid;
            try {
                uuid = UUID.fromString(minecraftUuid);
            } catch (IllegalArgumentException e) {
                return;
            }

            Player player = Bukkit.getPlayer(uuid);
            if (player == null) {
                return; // not online — enforcement at next login via HTTP check
            }

            switch (eventType) {
                case "punishment.issue" -> {
                    switch (type) {
                        case "BAN" -> Bukkit.getScheduler().runTask(plugin,
                                () -> player.kickPlayer("§cYou have been banned.\n§7Reason: " + reason));
                        case "MUTE" -> player.sendMessage("§cYou have been muted. Reason: " + reason);
                        case "KICK" -> Bukkit.getScheduler().runTask(plugin,
                                () -> player.kickPlayer("§eYou have been kicked.\n§7Reason: " + reason));
                        case "WARN" -> player.sendMessage("§eWarning: " + reason);
                    }
                }
                case "punishment.revoke" -> {
                    switch (type) {
                        case "BAN" -> plugin.getLogger().info("[EventStream] Ban revoked for " + minecraftUuid + " (they are offline or already removed)");
                        case "MUTE" -> player.sendMessage("§aYour mute has been removed.");
                    }
                }
            }
        } catch (Exception e) {
            plugin.getLogger().warning("[EventStream] Failed to handle event: " + e.getMessage());
        }
    }

    private void sleep() {
        try {
            Thread.sleep(RECONNECT_DELAY_MS);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
    }
}
