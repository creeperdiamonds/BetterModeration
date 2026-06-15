package xyz.creeperdiamonds.bettermoderation.paper.listeners;

import xyz.creeperdiamonds.bettermoderation.core.domain.Punishment;
import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;
import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;
import org.bukkit.event.EventHandler;
import org.bukkit.event.EventPriority;
import org.bukkit.event.Listener;
import org.bukkit.event.player.PlayerLoginEvent;

import java.time.Instant;
import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.List;

public class PlayerJoinListener implements Listener {

    private static final DateTimeFormatter DATE_FORMAT =
            DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm 'UTC'").withZone(ZoneId.of("UTC"));

    private final BetterModerationPlugin plugin;

    public PlayerJoinListener(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @EventHandler(priority = EventPriority.LOWEST)
    public void onPlayerLogin(PlayerLoginEvent event) {
        String uuid = event.getPlayer().getUniqueId().toString();
        String ip = event.getAddress() != null ? event.getAddress().getHostAddress() : null;

        // Fetch punishments (backend also tracks IP and enforces IP bans)
        List<Punishment> punishments;
        try {
            punishments = plugin.getBackendClient()
                    .getActivePunishments(uuid, ip)
                    .get(4, java.util.concurrent.TimeUnit.SECONDS);
        } catch (Exception e) {
            plugin.getLogger().warning("Could not fetch punishments for " + uuid + ": " + e.getMessage());
            return;
        }

        if (punishments == null) return;

        for (Punishment punishment : punishments) {
            if (!punishment.isActive() || punishment.isExpired()) continue;

            if (punishment.getType() == PunishmentType.BAN) {
                String expiry = punishment.getExpiresAt() == null
                        ? "permanent"
                        : DATE_FORMAT.format(Instant.ofEpochMilli(punishment.getExpiresAt()));

                String message = "§cYou are banned from this server.\n"
                        + "§7Reason: §f" + punishment.getReason() + "\n"
                        + "§7Expires: §f" + expiry + "\n"
                        + "§7Appeal at: §bhttps://bettermoderation.dev/appeal";

                event.disallow(PlayerLoginEvent.Result.KICK_BANNED, net.kyori.adventure.text.Component.text(message));
                return;
            }
        }

        // Alt detection: kick ban evaders whose alternate accounts are banned
        try {
            boolean altBanned = plugin.getBackendClient()
                    .hasAltWithActiveBan(uuid)
                    .get(3, java.util.concurrent.TimeUnit.SECONDS);
            if (altBanned) {
                plugin.getLogger().warning("[BetterModeration] Blocking potential ban evasion: " + uuid);
                event.disallow(PlayerLoginEvent.Result.KICK_BANNED,
                        net.kyori.adventure.text.Component.text(
                                "§cYou are banned from this server (ban evasion).\n"
                                + "§7Appeal at: §bhttps://bettermoderation.dev/appeal"));
            }
        } catch (Exception e) {
            plugin.getLogger().warning("Could not check alts for " + uuid + ": " + e.getMessage());
        }
    }
}
