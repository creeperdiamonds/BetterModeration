package xyz.creeperdiamonds.bettermoderation.paper.commands;

import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;
import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;
import org.bukkit.Bukkit;
import org.bukkit.command.Command;
import org.bukkit.command.CommandExecutor;
import org.bukkit.command.CommandSender;
import org.bukkit.entity.Player;

/**
 * /kick <player> [reason...]
 */
public class KickCommand implements CommandExecutor {

    private final BetterModerationPlugin plugin;

    public KickCommand(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @Override
    public boolean onCommand(CommandSender sender, Command command, String label, String[] args) {
        if (!sender.hasPermission("bettermoderation.kick")) {
            sender.sendMessage("§cYou do not have permission to kick players.");
            return true;
        }

        if (args.length < 1) {
            sender.sendMessage("§cUsage: /kick <player> [reason]");
            return true;
        }

        String targetName = args[0];
        Player target = Bukkit.getPlayer(targetName);
        if (target == null) {
            sender.sendMessage("§cPlayer §f" + targetName + "§c is not online.");
            return true;
        }

        String reason = args.length > 1
                ? String.join(" ", java.util.Arrays.copyOfRange(args, 1, args.length))
                : "No reason provided";

        String issuedBy = sender instanceof Player ? sender.getName() : "CONSOLE";
        String uuid = target.getUniqueId().toString();

        // Kick immediately (must be on main thread; CommandExecutor runs on main thread)
        target.kickPlayer("§cYou have been kicked.\n§7Reason: §f" + reason);

        // Log the kick to the backend asynchronously
        plugin.getBackendClient()
                .issuePunishment(uuid, PunishmentType.KICK, reason, issuedBy, null)
                .thenAccept(punishment -> {
                    if (punishment != null) {
                        plugin.getLogger().info("Logged kick for " + targetName + " (reason: " + reason + ")");
                    }
                })
                .exceptionally(ex -> {
                    plugin.getLogger().warning("Failed to log kick for " + uuid + ": " + ex.getMessage());
                    return null;
                });

        sender.sendMessage("§aKicked §f" + targetName + " §afor: §f" + reason);
        return true;
    }
}
