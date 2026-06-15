package xyz.creeperdiamonds.bettermoderation.velocity.listeners;

import com.velocitypowered.api.event.ResultedEvent;
import com.velocitypowered.api.event.Subscribe;
import com.velocitypowered.api.event.connection.LoginEvent;
import xyz.creeperdiamonds.bettermoderation.core.domain.Punishment;
import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;
import xyz.creeperdiamonds.bettermoderation.velocity.BetterModerationVelocity;
import xyz.creeperdiamonds.bettermoderation.velocity.sync.BackendClient;
import net.kyori.adventure.text.Component;

import java.time.Instant;
import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.List;
import java.util.concurrent.TimeUnit;

/**
 * Checks for active bans on the LoginEvent (post-auth), where the player's UUID is
 * available. Using LoginEvent rather than PreLoginEvent ensures UUID-based lookup
 * works correctly for both online and offline-mode servers.
 */
public class PlayerConnectListener {

    private static final DateTimeFormatter DATE_FORMAT =
            DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm 'UTC'").withZone(ZoneId.of("UTC"));

    private final BetterModerationVelocity plugin;
    private final BackendClient backendClient;

    public PlayerConnectListener(BetterModerationVelocity plugin, BackendClient backendClient) {
        this.plugin = plugin;
        this.backendClient = backendClient;
    }

    @Subscribe
    public void onLogin(LoginEvent event) {
        String uuid = event.getPlayer().getUniqueId().toString();
        String ip = event.getPlayer().getRemoteAddress() != null
                ? event.getPlayer().getRemoteAddress().getAddress().getHostAddress()
                : null;

        List<Punishment> punishments;
        try {
            punishments = backendClient.getActivePunishments(uuid, ip)
                    .get(4, TimeUnit.SECONDS);
        } catch (Exception e) {
            plugin.getLogger().warn("Could not fetch punishments for {}: {}", uuid, e.getMessage());
            return;
        }

        if (punishments == null) return;

        for (Punishment punishment : punishments) {
            if (!punishment.isActive() || punishment.isExpired()) continue;

            if (punishment.getType() == PunishmentType.BAN) {
                String expiry = punishment.getExpiresAt() == null
                        ? "permanent"
                        : DATE_FORMAT.format(Instant.ofEpochMilli(punishment.getExpiresAt()));

                Component denyMessage = Component.text(
                        "§cYou are banned from this network.\n"
                        + "§7Reason: §f" + punishment.getReason() + "\n"
                        + "§7Expires: §f" + expiry + "\n"
                        + "§7Appeal at: §bhttps://bettermoderation.dev/appeal"
                );

                event.setResult(ResultedEvent.ComponentResult.denied(denyMessage));
                return;
            }
        }

        // Alt detection: kick ban evaders whose alternate accounts are banned
        try {
            boolean altBanned = backendClient.hasAltWithActiveBan(uuid)
                    .get(3, TimeUnit.SECONDS);
            if (altBanned) {
                plugin.getLogger().warn("[BetterModeration] Blocking potential ban evasion: {}", uuid);
                event.setResult(ResultedEvent.ComponentResult.denied(Component.text(
                        "§cYou are banned from this network (ban evasion).\n"
                        + "§7Appeal at: §bhttps://bettermoderation.dev/appeal"
                )));
            }
        } catch (Exception e) {
            plugin.getLogger().warn("Could not check alts for {}: {}", uuid, e.getMessage());
        }
    }
}
