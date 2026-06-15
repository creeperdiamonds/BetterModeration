package xyz.creeperdiamonds.bettermoderation.paper.commands;

import xyz.creeperdiamonds.bettermoderation.paper.BetterModerationPlugin;
import org.bukkit.command.Command;
import org.bukkit.command.CommandExecutor;
import org.bukkit.command.CommandSender;
import org.bukkit.entity.Player;

/**
 * /link — Generates a one-time link code and sends it to the player.
 */
public class LinkCommand implements CommandExecutor {

    private final BetterModerationPlugin plugin;

    public LinkCommand(BetterModerationPlugin plugin) {
        this.plugin = plugin;
    }

    @Override
    public boolean onCommand(CommandSender sender, Command command, String label, String[] args) {
        if (!(sender instanceof Player player)) {
            sender.sendMessage("§cThis command can only be used by a player.");
            return true;
        }

        player.sendMessage("§7Generating your link code, please wait...");

        String uuid = player.getUniqueId().toString();

        plugin.getBackendClient()
                .generateLinkCode(uuid)
                .thenAccept(code -> {
                    if (code == null || code.isBlank()) {
                        player.sendMessage("§cCould not generate a link code. Is your account already linked?");
                        return;
                    }
                    player.sendMessage(
                            "§aYour BetterModeration link code is:\n"
                            + "§b§l" + code + "\n"
                            + "§7Visit §nhttps://bettermoderation.dev/link §7and enter this code to link\n"
                            + "§7your Minecraft account to your Discord. Code expires in 10 minutes."
                    );
                })
                .exceptionally(ex -> {
                    player.sendMessage("§cFailed to contact the backend: " + ex.getMessage());
                    return null;
                });

        return true;
    }
}
