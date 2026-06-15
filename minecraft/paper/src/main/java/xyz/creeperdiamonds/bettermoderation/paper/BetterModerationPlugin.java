package xyz.creeperdiamonds.bettermoderation.paper;

import xyz.creeperdiamonds.bettermoderation.paper.commands.BanCommand;
import xyz.creeperdiamonds.bettermoderation.paper.commands.KickCommand;
import xyz.creeperdiamonds.bettermoderation.paper.commands.LinkCommand;
import xyz.creeperdiamonds.bettermoderation.paper.commands.MuteCommand;
import xyz.creeperdiamonds.bettermoderation.paper.commands.ReportCommand;
import xyz.creeperdiamonds.bettermoderation.paper.commands.WarnCommand;
import xyz.creeperdiamonds.bettermoderation.paper.listeners.PlayerChatListener;
import xyz.creeperdiamonds.bettermoderation.paper.listeners.PlayerJoinListener;
import xyz.creeperdiamonds.bettermoderation.paper.listeners.PlayerQuitListener;
import xyz.creeperdiamonds.bettermoderation.paper.sync.BackendClient;
import xyz.creeperdiamonds.bettermoderation.paper.sync.EventStreamClient;
import org.bukkit.plugin.java.JavaPlugin;

public final class BetterModerationPlugin extends JavaPlugin {

    private static BetterModerationPlugin instance;
    private BackendClient backendClient;
    private EventStreamClient eventStreamClient;
    private Thread eventStreamThread;

    @Override
    public void onEnable() {
        instance = this;

        // Save and load default config
        saveDefaultConfig();
        String backendUrl = getConfig().getString("backend-url", "http://localhost:8080");
        String serverId   = getConfig().getString("server-id", "unknown");
        String apiKey     = getConfig().getString("api-key", "");

        // Initialize backend client
        backendClient = new BackendClient(backendUrl, serverId, apiKey);

        // Start SSE event stream for real-time punishment enforcement
        eventStreamClient = new EventStreamClient(this, backendUrl, serverId, apiKey);
        eventStreamThread = new Thread(eventStreamClient, "bm-event-stream");
        eventStreamThread.setDaemon(true);
        eventStreamThread.start();

        // Register listeners
        getServer().getPluginManager().registerEvents(new PlayerJoinListener(this), this);
        getServer().getPluginManager().registerEvents(new PlayerChatListener(this), this);
        getServer().getPluginManager().registerEvents(new PlayerQuitListener(this), this);

        // Register commands
        getCommand("ban").setExecutor(new BanCommand(this));
        getCommand("mute").setExecutor(new MuteCommand(this));
        getCommand("kick").setExecutor(new KickCommand(this));
        getCommand("warn").setExecutor(new WarnCommand(this));
        getCommand("link").setExecutor(new LinkCommand(this));
        getCommand("report").setExecutor(new ReportCommand(this));

        getLogger().info("BetterModeration enabled — connected to " + backendUrl);
    }

    @Override
    public void onDisable() {
        if (eventStreamClient != null) {
            eventStreamClient.stop();
        }
        if (backendClient != null) {
            backendClient.shutdown();
        }
        getLogger().info("BetterModeration disabled.");
    }

    public static BetterModerationPlugin getInstance() {
        return instance;
    }

    public BackendClient getBackendClient() {
        return backendClient;
    }
}
