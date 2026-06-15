package xyz.creeperdiamonds.bettermoderation.paper.commands;

import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;
import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;
import org.bukkit.Bukkit;
import org.bukkit.command.Command;
import org.bukkit.command.CommandExecutor;
import org.bukkit.command.CommandSender;
import org.bukkit.entity.Player;

/**
 * /ban <player> [duration] [reason...]
 *
 * Duration formats: 1h, 7d, 2w, 1mo, 1y, forever
 */
public class BanCommand implements CommandExecutor {

    private final BetterModerationPlugin plugin;

    public BanCommand(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @Override
    public boolean onCommand(CommandSender sender, Command command, String label, String[] args) {
        if (!sender.hasPermission("bettermoderation.ban")) {
            sender.sendMessage("§cYou do not have permission to ban players.");
            return true;
        }

        if (args.length < 1) {
            sender.sendMessage("§cUsage: /ban <player> [duration] [reason]");
            return true;
        }

        String targetName = args[0];
        Player target = Bukkit.getPlayer(targetName);
        if (target == null) {
            sender.sendMessage("§cPlayer §f" + targetName + "§c is not online.");
            return true;
        }

        Long expiresAt = null;
        int reasonStart = 1;

        if (args.length >= 2 && looksLikeDuration(args[1])) {
            long durationMs = parseDuration(args[1]);
            if (durationMs < 0) {
                expiresAt = null; // permanent
            } else {
                expiresAt = System.currentTimeMillis() + durationMs;
            }
            reasonStart = 2;
        }

        String reason = reasonStart < args.length
                ? String.join(" ", java.util.Arrays.copyOfRange(args, reasonStart, args.length))
                : "No reason provided";

        String issuedBy = sender instanceof Player ? sender.getName() : "CONSOLE";
        String uuid = target.getUniqueId().toString();
        final Long finalExpiresAt = expiresAt;

        sender.sendMessage("§aBanning §f" + targetName + "§a...");

        plugin.getBackendClient()
                .issuePunishment(uuid, PunishmentType.BAN, reason, issuedBy, finalExpiresAt)
                .thenAccept(punishment -> {
                    if (punishment == null) {
                        sender.sendMessage("§cFailed to ban §f" + targetName + "§c — backend error.");
                        return;
                    }
                    String expStr = finalExpiresAt == null ? "permanent" : "expires at " + finalExpiresAt;
                    sender.sendMessage("§aBanned §f" + targetName + " §afor: §f" + reason + " §7(" + expStr + ")");
                    target.kickPlayer("§cYou have been banned.\n§7Reason: §f" + reason);
                    Bukkit.broadcastMessage("§c" + targetName + " has been banned by " + issuedBy + ".");
                })
                .exceptionally(ex -> {
                    sender.sendMessage("§cError: " + ex.getMessage());
                    return null;
                });

        return true;
    }

    private boolean looksLikeDuration(String s) {
        return s.matches("\\d+[hdwmy]o?|forever");
    }

    /**
     * Parses duration strings like 1h, 7d, 2w, 1mo, 1y, forever.
     * Returns -1 for permanent (forever).
     */
    private long parseDuration(String s) {
        if (s.equalsIgnoreCase("forever")) return -1L;
        if (s.endsWith("mo")) {
            long n = Long.parseLong(s.substring(0, s.length() - 2));
            return n * 30L * 24 * 60 * 60 * 1000;
        }
        char unit = s.charAt(s.length() - 1);
        long n = Long.parseLong(s.substring(0, s.length() - 1));
        return switch (unit) {
            case 'h' -> n * 3600_000L;
            case 'd' -> n * 86400_000L;
            case 'w' -> n * 604800_000L;
            case 'y' -> n * 31536000_000L;
            default  -> n * 60_000L; // treat unknown as minutes
        };
    }
}
