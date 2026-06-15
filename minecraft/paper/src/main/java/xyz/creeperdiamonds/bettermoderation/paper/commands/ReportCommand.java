package xyz.creeperdiamonds.bettermoderation.paper.commands;

import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;
import org.bukkit.Bukkit;
import org.bukkit.command.Command;
import org.bukkit.command.CommandExecutor;
import org.bukkit.command.CommandSender;
import org.bukkit.entity.Player;

import java.util.Arrays;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

/**
 * /report <player> <reason...>
 */
public class ReportCommand implements CommandExecutor {

    // Local cooldown: prevent the same player from spamming /report within COOLDOWN_MS milliseconds.
    private static final long COOLDOWN_MS = 30_000;
    private static final int MAX_REASON_LENGTH = 500;

    private final BetterModerationPlugin plugin;
    // key: reporter UUID, value: last report timestamp (ms)
    private final Map<UUID, Long> lastReport = new ConcurrentHashMap<>();

    public ReportCommand(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @Override
    public boolean onCommand(CommandSender sender, Command command, String label, String[] args) {
        if (!(sender instanceof Player reporter)) {
            sender.sendMessage("§cThis command can only be used by a player.");
            return true;
        }

        if (args.length < 2) {
            sender.sendMessage("§cUsage: /report <player> <reason>");
            return true;
        }

        // Local cooldown check
        long now = System.currentTimeMillis();
        Long last = lastReport.get(reporter.getUniqueId());
        if (last != null && now - last < COOLDOWN_MS) {
            long secsLeft = (COOLDOWN_MS - (now - last)) / 1000 + 1;
            reporter.sendMessage("§cYou must wait §f" + secsLeft + "s §cbefore submitting another report.");
            return true;
        }

        String targetName = args[0];
        Player target = Bukkit.getPlayer(targetName);
        if (target == null) {
            reporter.sendMessage("§cPlayer §f" + targetName + "§c is not online.");
            return true;
        }

        if (target.equals(reporter)) {
            reporter.sendMessage("§cYou cannot report yourself.");
            return true;
        }

        String reason = String.join(" ", Arrays.copyOfRange(args, 1, args.length));
        if (reason.length() > MAX_REASON_LENGTH) {
            reporter.sendMessage("§cReason is too long (max " + MAX_REASON_LENGTH + " characters).");
            return true;
        }

        // Record cooldown before sending to backend
        lastReport.put(reporter.getUniqueId(), now);

        String reporterUuid = reporter.getUniqueId().toString();
        String targetUuid = target.getUniqueId().toString();

        reporter.sendMessage("§7Submitting your report...");

        plugin.getBackendClient()
                .reportPlayer(reporterUuid, targetUuid, reason)
                .thenAccept(v -> reporter.sendMessage(
                        "§aYour report against §f" + targetName + " §ahas been submitted.\n"
                        + "§7Our moderation team will review it shortly. Thank you."))
                .exceptionally(ex -> {
                    // Backend rejected (rate limit, duplicate, etc.) — reset cooldown so they can retry
                    lastReport.remove(reporter.getUniqueId());
                    reporter.sendMessage("§cFailed to submit report: " + ex.getMessage());
                    return null;
                });

        return true;
    }
}
