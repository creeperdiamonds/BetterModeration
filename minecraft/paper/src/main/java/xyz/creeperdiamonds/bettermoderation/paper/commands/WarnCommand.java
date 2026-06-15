package xyz.creeperdiamonds.bettermoderation.paper.commands;

import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;
import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;
import org.bukkit.Bukkit;
import org.bukkit.command.Command;
import org.bukkit.command.CommandExecutor;
import org.bukkit.command.CommandSender;
import org.bukkit.entity.Player;

/**
 * /warn <player> <reason...>
 */
public class WarnCommand implements CommandExecutor {

    private final BetterModerationPlugin plugin;

    public WarnCommand(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @Override
    public boolean onCommand(CommandSender sender, Command command, String label, String[] args) {
        if (!sender.hasPermission("bettermoderation.warn")) {
            sender.sendMessage("§cYou do not have permission to warn players.");
            return true;
        }

        if (args.length < 2) {
            sender.sendMessage("§cUsage: /warn <player> <reason>");
            return true;
        }

        String targetName = args[0];
        Player target = Bukkit.getPlayer(targetName);
        if (target == null) {
            sender.sendMessage("§cPlayer §f" + targetName + "§c is not online.");
            return true;
        }

        String reason = String.join(" ", java.util.Arrays.copyOfRange(args, 1, args.length));
        String issuedBy = sender instanceof Player ? sender.getName() : "CONSOLE";
        String uuid = target.getUniqueId().toString();

        // Notify the warned player immediately in-game
        target.sendMessage(
                "§e§lYou have received a warning from " + issuedBy + ":\n"
                + "§f" + reason + "\n"
                + "§7Please review the server rules to avoid further action."
        );

        plugin.getBackendClient()
                .issuePunishment(uuid, PunishmentType.WARN, reason, issuedBy, null)
                .thenAccept(punishment -> {
                    if (punishment == null) {
                        sender.sendMessage("§cFailed to log warning for §f" + targetName + "§c — backend error.");
                        return;
                    }
                    sender.sendMessage("§aWarned §f" + targetName + " §afor: §f" + reason);
                })
                .exceptionally(ex -> {
                    sender.sendMessage("§cError logging warning: " + ex.getMessage());
                    return null;
                });

        return true;
    }
}
