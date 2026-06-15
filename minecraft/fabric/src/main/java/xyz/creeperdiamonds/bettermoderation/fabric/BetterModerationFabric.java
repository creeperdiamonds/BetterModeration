package xyz.creeperdiamonds.bettermoderation.fabric;

import xyz.creeperdiamonds.bettermoderation.core.domain.ConnectResponse;
import xyz.creeperdiamonds.bettermoderation.fabric.sync.BackendClient;
import xyz.creeperdiamonds.bettermoderation.fabric.sync.EventStreamClient;
import net.fabricmc.api.ModInitializer;
import net.fabricmc.fabric.api.event.lifecycle.v1.ServerLifecycleEvents;
import net.fabricmc.fabric.api.networking.v1.ServerPlayConnectionEvents;
import net.minecraft.server.network.ServerPlayerEntity;
import net.minecraft.text.Text;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.concurrent.TimeUnit;

public class BetterModerationFabric implements ModInitializer {

    public static final String MOD_ID = "bettermoderation";
    public static final Logger LOGGER = LoggerFactory.getLogger(MOD_ID);

    private static BetterModerationFabric instance;
    private BackendClient backendClient;
    private EventStreamClient eventStreamClient;
    private Thread eventStreamThread;

    @Override
    public void onInitialize() {
        instance = this;

        String backendUrl = getEnvOrDefault("BM_BACKEND_URL", "http://localhost:8080");
        String serverId   = getEnvOrDefault("BM_SERVER_ID", "fabric-server");
        String apiKey     = getEnvOrDefault("BM_API_KEY", "");

        backendClient = new BackendClient(backendUrl, serverId, apiKey);

        eventStreamClient = new EventStreamClient(LOGGER, backendUrl, serverId, apiKey);
        eventStreamThread = new Thread(eventStreamClient, "bm-event-stream");
        eventStreamThread.setDaemon(true);
        eventStreamThread.start();

        // Enforce bans and evasion detection on player join
        ServerPlayConnectionEvents.JOIN.register((handler, sender, server) -> {
            ServerPlayerEntity player = handler.player;
            String uuid = player.getUuid().toString();
            String username = player.getName().getString();

            String ip = (player.networkHandler.connection.getAddress() instanceof java.net.InetSocketAddress isa)
                    ? isa.getAddress().getHostAddress()
                    : null;
            boolean offline = !server.isOnlineMode();

            server.execute(() -> {
                ConnectResponse resp;
                try {
                    resp = backendClient.sessionConnect(uuid, username, ip, offline)
                            .get(5, TimeUnit.SECONDS);
                } catch (Exception e) {
                    LOGGER.warn("[BetterModeration] sessionConnect timed out for {}: {}", uuid, e.getMessage());
                    return; // fail-open
                }

                if (resp == null) return; // fail-open on error

                if (resp.getAction() == ConnectResponse.Action.DENY) {
                    String msg = resp.getKickMessage() != null
                            ? resp.getKickMessage()
                            : "§cYou are banned from this server.\n§7Appeal at: §bhttps://bettermoderation.dev/appeal";
                    player.networkHandler.disconnect(Text.literal(msg));
                    return;
                }
                // FLAG: backend handles Discord notification — plugin does nothing extra

                // Strip op from cracked players — on offline-mode servers op is stored by
                // username, so anyone can impersonate an opped player by choosing that name.
                if (offline && player.hasPermissionLevel(4)) {
                    server.getPlayerManager().removeFromOperators(player.getGameProfile());
                    LOGGER.warn("[BetterModeration] Stripped op from cracked player {} ({}) — op is disabled in offline mode.",
                            username, uuid);
                }
            });
        });

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
                backendClient.notifyServerStatus(false).get(3, TimeUnit.SECONDS);
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
