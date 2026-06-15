package xyz.creeperdiamonds.bettermoderation.paper.listeners;

import xyz.creeperdiamonds.bettermoderation.core.domain.Punishment;
import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;
import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;
import org.bukkit.event.EventHandler;
import org.bukkit.event.EventPriority;
import org.bukkit.event.Listener;
import org.bukkit.event.player.AsyncPlayerChatEvent;
import org.bukkit.event.player.PlayerQuitEvent;

import java.time.Instant;
import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.ArrayDeque;
import java.util.Deque;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.TimeUnit;

@SuppressWarnings("deprecation") // AsyncPlayerChatEvent is deprecated in later Paper versions but still valid
public class PlayerChatListener implements Listener {

    private static final DateTimeFormatter DATE_FORMAT =
            DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm 'UTC'").withZone(ZoneId.of("UTC"));

    // Rapid-fire throttle: max messages within the sliding window before silencing
    private static final int THROTTLE_MAX = 5;
    private static final long THROTTLE_WINDOW_MS = 3_000;

    private final BetterModerationPlugin plugin;
    // Tracks recent message timestamps per player for rapid-fire detection
    private final Map<UUID, Deque<Long>> messageTimestamps = new ConcurrentHashMap<>();

    public PlayerChatListener(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @EventHandler(priority = EventPriority.LOWEST, ignoreCancelled = true)
    public void onPlayerChat(AsyncPlayerChatEvent event) {
        // 1. Rapid-fire throttle — block bots/macro spam before hitting the backend
        if (isRapidFire(event.getPlayer().getUniqueId())) {
            event.setCancelled(true);
            event.getPlayer().sendMessage("§cYou are sending messages too quickly. Please slow down.");
            return;
        }

        // 2. Mute check via backend
        String uuid = event.getPlayer().getUniqueId().toString();
        List<Punishment> punishments;
        try {
            punishments = plugin.getBackendClient()
                    .getActivePunishments(uuid, null)
                    .get(3, TimeUnit.SECONDS);
        } catch (Exception e) {
            plugin.getLogger().warning("Could not check mute for " + uuid + ": " + e.getMessage());
            return;
        }

        if (punishments == null) return;

        for (Punishment punishment : punishments) {
            if (!punishment.isActive() || punishment.isExpired()) continue;

            if (punishment.getType() == PunishmentType.MUTE) {
                event.setCancelled(true);

                String expiry = punishment.getExpiresAt() == null
                        ? "permanent"
                        : DATE_FORMAT.format(Instant.ofEpochMilli(punishment.getExpiresAt()));

                event.getPlayer().sendMessage(
                        "§cYou are muted and cannot send messages.\n"
                        + "§7Reason: §f" + punishment.getReason() + "\n"
                        + "§7Expires: §f" + expiry
                );
                return;
            }
        }
    }

    @EventHandler
    public void onPlayerQuit(PlayerQuitEvent event) {
        messageTimestamps.remove(event.getPlayer().getUniqueId());
    }

    private boolean isRapidFire(UUID uuid) {
        long now = System.currentTimeMillis();
        Deque<Long> times = messageTimestamps.computeIfAbsent(uuid, k -> new ArrayDeque<>());

        // Evict timestamps outside the window
        while (!times.isEmpty() && now - times.peekFirst() > THROTTLE_WINDOW_MS) {
            times.pollFirst();
        }

        times.addLast(now);
        return times.size() > THROTTLE_MAX;
    }
}
