package xyz.creeperdiamonds.bettermoderation.paper.listeners;

import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;
import org.bukkit.event.EventHandler;
import org.bukkit.event.Listener;
import org.bukkit.event.player.PlayerQuitEvent;

public class PlayerQuitListener implements Listener {

    private final BetterModerationPlugin plugin;

    public PlayerQuitListener(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @EventHandler
    public void onPlayerQuit(PlayerQuitEvent event) {
        String uuid = event.getPlayer().getUniqueId().toString();

        // Fire-and-forget async session tracking — do not block the quit event
        plugin.getBackendClient()
                .notifyDisconnect(uuid)
                .exceptionally(ex -> {
                    plugin.getLogger().warning("Failed to log disconnect for " + uuid + ": " + ex.getMessage());
                    return null;
                });
    }
}
