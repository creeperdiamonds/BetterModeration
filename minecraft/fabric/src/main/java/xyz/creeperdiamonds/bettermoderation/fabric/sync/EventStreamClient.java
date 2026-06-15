package xyz.creeperdiamonds.bettermoderation.fabric.sync;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import net.minecraft.server.MinecraftServer;
import net.minecraft.server.network.ServerPlayerEntity;
import net.minecraft.text.Text;
import org.slf4j.Logger;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.UUID;

/**
 * Connects to the backend SSE stream and handles real-time punishment events.
 * Runs on a daemon thread — the server does not need a Redis connection.
 */
public class EventStreamClient implements Runnable {

    private static final long RECONNECT_DELAY_MS = 5_000;

    private final Logger logger;
    private final String streamUrl;
    private final String serverId;
    private final String apiKey;
    private final ObjectMapper mapper = new ObjectMapper();

    private volatile MinecraftServer server;
    private volatile boolean running = true;

    public EventStreamClient(Logger logger, String baseUrl, String serverId, String apiKey) {
        this.logger = logger;
        this.streamUrl = (baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl)
                + "/v1/events/stream";
        this.serverId = serverId;
        this.apiKey = apiKey;
    }

    public void setServer(MinecraftServer server) {
        this.server = server;
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
                    logger.warn("[EventStream] Backend returned {}, retrying...", response.statusCode());
                    sleep();
                    continue;
                }

                logger.info("[EventStream] Connected to backend event stream.");

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
                    }
                }

                logger.info("[EventStream] Connection closed, reconnecting...");

            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return;
            } catch (Exception e) {
                logger.warn("[EventStream] Error: {}, reconnecting...", e.getMessage());
            }
            sleep();
        }
    }

    private void handleEvent(String eventType, String data) {
        MinecraftServer srv = server;
        if (srv == null) {
            return;
        }

        try {
            JsonNode event = mapper.readTree(data);
            String minecraftUuid = event.path("minecraft_uuid").asText(null);
            String type = event.path("type").asText();
            String reason = event.path("reason").asText("You have been " + type.toLowerCase() + ".");

            if (minecraftUuid == null || minecraftUuid.isBlank()) {
                return;
            }

            UUID uuid;
            try {
                uuid = UUID.fromString(minecraftUuid);
            } catch (IllegalArgumentException e) {
                return;
            }

            ServerPlayerEntity player = srv.getPlayerManager().getPlayer(uuid);
            if (player == null) {
                return;
            }

            switch (eventType) {
                case "punishment.issue" -> {
                    switch (type) {
                        case "BAN" -> srv.execute(() -> player.networkHandler.disconnect(
                                Text.literal("§cYou have been banned.\n§7Reason: " + reason)));
                        case "KICK" -> srv.execute(() -> player.networkHandler.disconnect(
                                Text.literal("§eYou have been kicked.\n§7Reason: " + reason)));
                        case "MUTE" -> player.sendMessage(Text.literal("§cYou have been muted. Reason: " + reason));
                        case "WARN" -> player.sendMessage(Text.literal("§eWarning: " + reason));
                    }
                }
                case "punishment.revoke" -> {
                    if ("MUTE".equals(type)) {
                        player.sendMessage(Text.literal("§aYour mute has been removed."));
                    }
                }
            }
        } catch (Exception e) {
            logger.warn("[EventStream] Failed to handle event: {}", e.getMessage());
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
