package xyz.creeperdiamonds.bettermoderation.fabric;

import xyz.creeperdiamonds.bettermoderation.core.domain.Punishment;
import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;
import xyz.creeperdiamonds.bettermoderation.fabric.sync.BackendClient;
import xyz.creeperdiamonds.bettermoderation.fabric.sync.EventStreamClient;
import net.fabricmc.api.ModInitializer;
import net.fabricmc.fabric.api.event.lifecycle.v1.ServerLifecycleEvents;
import net.fabricmc.fabric.api.networking.v1.ServerPlayConnectionEvents;
import net.minecraft.network.packet.s2c.play.DisconnectS2CPacket;
import net.minecraft.server.network.ServerPlayerEntity;
import net.minecraft.text.Text;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.time.Instant;
import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.List;

public class BetterModerationFabric implements ModInitializer {

    public static final String MOD_ID = "bettermoderation";
    public static final Logger LOGGER = LoggerFactory.getLogger(MOD_ID);

    private static final DateTimeFormatter DATE_FORMAT =
            DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm 'UTC'").withZone(ZoneId.of("UTC"));

    private static BetterModerationFabric instance;
    private BackendClient backendClient;
    private EventStreamClient eventStreamClient;
    private Thread eventStreamThread;

    @Override
    public void onInitialize() {
        instance = this;

        // Read configuration from environment variables (or fall back to defaults)
        String backendUrl = getEnvOrDefault("BM_BACKEND_URL", "http://localhost:8080");
        String serverId   = getEnvOrDefault("BM_SERVER_ID", "fabric-server");
        String apiKey     = getEnvOrDefault("BM_API_KEY", "");

        backendClient = new BackendClient(backendUrl, serverId, apiKey);

        // Start SSE event stream for real-time punishment enforcement
        eventStreamClient = new EventStreamClient(LOGGER, backendUrl, serverId, apiKey);
        eventStreamThread = new Thread(eventStreamClient, "bm-event-stream");
        eventStreamThread.setDaemon(true);
        eventStreamThread.start();

        // Enforce bans + alt detection on player join
        ServerPlayConnectionEvents.JOIN.register((handler, sender, server) -> {
            ServerPlayerEntity player = handler.player;
            String uuid = player.getUuid().toString();

            String ip = (player.networkHandler.connection.getAddress() instanceof java.net.InetSocketAddress isa)
                    ? isa.getAddress().getHostAddress()
                    : null;

            server.execute(() -> {
                // Check direct ban and IP ban (backend also tracks the IP)
                List<Punishment> punishments;
                try {
                    punishments = backendClient.getActivePunishments(uuid, ip)
                            .get(4, java.util.concurrent.TimeUnit.SECONDS);
                } catch (Exception e) {
                    LOGGER.warn("Could not fetch punishments for {}: {}", uuid, e.getMessage());
                    punishments = null;
                }

                if (punishments != null) {
                    for (Punishment punishment : punishments) {
                        if (!punishment.isActive() || punishment.isExpired()) continue;
                        if (punishment.getType() == PunishmentType.BAN) {
                            String expiry = punishment.getExpiresAt() == null
                                    ? "permanent"
                                    : DATE_FORMAT.format(Instant.ofEpochMilli(punishment.getExpiresAt()));
                            player.networkHandler.disconnect(Text.literal(
                                    "§cYou are banned from this server.\n"
                                    + "§7Reason: §f" + punishment.getReason() + "\n"
                                    + "§7Expires: §f" + expiry + "\n"
                                    + "§7Appeal at: §bhttps://bettermoderation.dev/appeal"
                            ));
                            return;
                        }
                    }
                }

                // Alt detection
                try {
                    boolean altBanned = backendClient.hasAltWithActiveBan(uuid)
                            .get(3, java.util.concurrent.TimeUnit.SECONDS);
                    if (altBanned) {
                        LOGGER.warn("[BetterModeration] Blocking potential ban evasion: {}", uuid);
                        player.networkHandler.disconnect(Text.literal(
                                "§cYou are banned from this server (ban evasion).\n"
                                + "§7Appeal at: §bhttps://bettermoderation.dev/appeal"
                        ));
                    }
                } catch (Exception e) {
                    LOGGER.warn("Could not check alts for {}: {}", uuid, e.getMessage());
                }
            });
        });

        // Register server lifecycle hooks
        ServerLifecycleEvents.SERVER_STARTED.register(server -> {
            eventStreamClient.setServer(server);
            LOGGER.info("BetterModeration: server started — notifying backend.");
            backendClient.notifyServerStatus(true).exceptionally(ex -> {
                LOGGER.warn("Failed to notify backend of server start: {}", ex.getMessage());
                return null;
            });
        });

        ServerLifecycleEvents.SERVER_STOPPING.register(server -> {
            LOGGER.info("BetterModeration: server stopping — notifying backend.");
            eventStreamClient.stop();
            try {
                backendClient.notifyServerStatus(false).get(3, java.util.concurrent.TimeUnit.SECONDS);
            } catch (Exception e) {
                LOGGER.warn("Failed to notify backend of server stop: {}", e.getMessage());
            }
        });

        LOGGER.info("BetterModeration Fabric mod initialized — backend: {}", backendUrl);
    }

    public static BetterModerationFabric getInstance() {
        return instance;
    }

    public BackendClient getBackendClient() {
        return backendClient;
    }

    private static String getEnvOrDefault(String key, String fallback) {
        String val = System.getenv(key);
        return (val != null && !val.isBlank()) ? val : fallback;
    }
}
