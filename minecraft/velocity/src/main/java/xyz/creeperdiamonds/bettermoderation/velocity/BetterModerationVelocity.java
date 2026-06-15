package xyz.creeperdiamonds.bettermoderation.velocity;

import com.google.inject.Inject;
import com.velocitypowered.api.event.Subscribe;
import com.velocitypowered.api.event.proxy.ProxyInitializeEvent;
import com.velocitypowered.api.event.proxy.ProxyShutdownEvent;
import com.velocitypowered.api.plugin.Plugin;
import com.velocitypowered.api.proxy.ProxyServer;
import xyz.creeperdiamonds.bettermoderation.velocity.listeners.PlayerConnectListener;
import xyz.creeperdiamonds.bettermoderation.velocity.sync.BackendClient;
import xyz.creeperdiamonds.bettermoderation.velocity.sync.EventStreamClient;
import org.slf4j.Logger;

@Plugin(
        id = "bettermoderation",
        name = "BetterModeration",
        version = "1.0.0",
        description = "Smart cross-platform moderation for Discord and Minecraft",
        authors = {"creeper_diamonds"}
)
public class BetterModerationVelocity {

    private final ProxyServer server;
    private final Logger logger;
    private BackendClient backendClient;
    private EventStreamClient eventStreamClient;
    private Thread eventStreamThread;

    @Inject
    public BetterModerationVelocity(ProxyServer server, Logger logger) {
        this.server = server;
        this.logger = logger;
    }

    @Subscribe
    public void onProxyInitialize(ProxyInitializeEvent event) {
        // Read config from system properties or environment variables
        String backendUrl = System.getenv().getOrDefault("BM_BACKEND_URL", "http://localhost:8080");
        String serverId   = System.getenv().getOrDefault("BM_SERVER_ID", "velocity-proxy");
        String apiKey     = System.getenv().getOrDefault("BM_API_KEY", "");

        backendClient = new BackendClient(backendUrl, serverId, apiKey);

        // Start SSE event stream for real-time punishment enforcement
        eventStreamClient = new EventStreamClient(server, logger, backendUrl, serverId, apiKey);
        eventStreamThread = new Thread(eventStreamClient, "bm-event-stream");
        eventStreamThread.setDaemon(true);
        eventStreamThread.start();

        // Register event listeners
        server.getEventManager().register(this, new PlayerConnectListener(this, backendClient));

        logger.info("BetterModeration Velocity plugin enabled — backend: {}", backendUrl);
    }

    @Subscribe
    public void onProxyShutdown(ProxyShutdownEvent event) {
        if (eventStreamClient != null) {
            eventStreamClient.stop();
        }
        logger.info("BetterModeration Velocity plugin disabled.");
    }

    public ProxyServer getServer() {
        return server;
    }

    public Logger getLogger() {
        return logger;
    }

    public BackendClient getBackendClient() {
        return backendClient;
    }
}
