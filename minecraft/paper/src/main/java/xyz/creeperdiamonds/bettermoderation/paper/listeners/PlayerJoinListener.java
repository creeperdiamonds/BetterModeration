package xyz.creeperdiamonds.bettermoderation.paper.listeners;

import xyz.creeperdiamonds.bettermoderation.core.domain.ConnectResponse;
import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;
import org.bukkit.Bukkit;
import org.bukkit.event.EventHandler;
import org.bukkit.event.EventPriority;
import org.bukkit.event.Listener;
import org.bukkit.event.player.PlayerLoginEvent;

import java.util.concurrent.TimeUnit;

public class PlayerJoinListener implements Listener {

    private final BetterModerationPlugin plugin;

    public PlayerJoinListener(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @EventHandler(priority = EventPriority.LOWEST)
    public void onPlayerLogin(PlayerLoginEvent event) {
        String uuid = event.getPlayer().getUniqueId().toString();
        String username = event.getPlayer().getName();
        String ip = event.getAddress() != null ? event.getAddress().getHostAddress() : null;
        boolean offline = !Bukkit.getServer().getOnlineMode();

        ConnectResponse resp;
        try {
            resp = plugin.getBackendClient()
                    .sessionConnect(uuid, username, ip, offline)
                    .get(5, TimeUnit.SECONDS);
        } catch (Exception e) {
            plugin.getLogger().warning("[BetterModeration] sessionConnect timed out for " + uuid + ": " + e.getMessage());
            return; // fail-open
        }

        if (resp == null) return; // fail-open on error

        if (resp.getAction() == ConnectResponse.Action.DENY) {
            String msg = resp.getKickMessage() != null
                    ? resp.getKickMessage()
                    : "§cYou are banned from this server.\n§7Appeal at: §bhttps://bettermoderation.dev/appeal";
            event.disallow(PlayerLoginEvent.Result.KICK_BANNED,
                    net.kyori.adventure.text.Component.text(msg));
        }
        // FLAG: backend handles Discord notification — plugin does nothing extra
    }
}
