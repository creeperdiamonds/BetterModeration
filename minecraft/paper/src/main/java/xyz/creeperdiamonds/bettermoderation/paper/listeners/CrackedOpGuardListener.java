package xyz.creeperdiamonds.bettermoderation.paper.listeners;

import org.bukkit.Bukkit;
import org.bukkit.event.EventHandler;
import org.bukkit.event.EventPriority;
import org.bukkit.event.Listener;
import org.bukkit.event.player.PlayerCommandPreprocessEvent;
import org.bukkit.event.player.PlayerJoinEvent;
import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;

/**
 * On offline-mode servers, op is stored by username — anyone can impersonate an
 * opped player simply by setting their username. This listener strips op on join
 * and blocks /op and /deop so the privilege can never be re-granted in-game.
 * Use a permissions plugin (e.g. LuckPerms) for admin access on cracked servers.
 */
public class CrackedOpGuardListener implements Listener {

    private final BetterModerationPlugin plugin;

    public CrackedOpGuardListener(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @EventHandler(priority = EventPriority.HIGH)
    public void onPlayerJoin(PlayerJoinEvent event) {
        if (Bukkit.getServer().getOnlineMode()) return;
        if (!event.getPlayer().isOp()) return;

        event.getPlayer().setOp(false);
        plugin.getLogger().warning(
            "[BetterModeration] Stripped op from cracked player " + event.getPlayer().getName()
            + " (" + event.getPlayer().getUniqueId() + ") — op is disabled in offline mode."
        );
    }

    @EventHandler(priority = EventPriority.LOWEST, ignoreCancelled = true)
    public void onCommand(PlayerCommandPreprocessEvent event) {
        if (Bukkit.getServer().getOnlineMode()) return;

        String cmd = event.getMessage().toLowerCase().trim();
        if (cmd.startsWith("/op ") || cmd.equals("/op")
                || cmd.startsWith("/minecraft:op ")
                || cmd.startsWith("/deop ") || cmd.equals("/deop")
                || cmd.startsWith("/minecraft:deop ")) {
            event.setCancelled(true);
            event.getPlayer().sendMessage(
                "§c[BetterModeration] /op and /deop are disabled on offline-mode servers.\n"
                + "§7Use a permissions plugin to manage admin access."
            );
        }
    }
}
