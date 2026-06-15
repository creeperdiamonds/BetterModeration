package xyz.creeperdiamonds.bettermoderation.velocity.listeners;

import com.velocitypowered.api.event.ResultedEvent;
import com.velocitypowered.api.event.Subscribe;
import com.velocitypowered.api.event.connection.LoginEvent;
import xyz.creeperdiamonds.bettermoderation.core.domain.ConnectResponse;
import xyz.creeperdiamonds.bettermoderation.velocity.BetterModerationVelocity;
import xyz.creeperdiamonds.bettermoderation.velocity.sync.BackendClient;
import net.kyori.adventure.text.Component;

import java.util.concurrent.TimeUnit;

/**
 * Checks for bans and evasion signals on LoginEvent (post-auth), where the
 * player's UUID is available. Uses the unified /v1/sessions/connect endpoint
 * which combines tracking, scoring, and ban enforcement in one call.
 */
public class PlayerConnectListener {

    private final BetterModerationVelocity plugin;
    private final BackendClient backendClient;

    public PlayerConnectListener(BetterModerationVelocity plugin, BackendClient backendClient) {
        this.plugin = plugin;
        this.backendClient = backendClient;
    }

    @Subscribe
    public void onLogin(LoginEvent event) {
        String uuid = event.getPlayer().getUniqueId().toString();
        String username = event.getPlayer().getUsername();
        String ip = event.getPlayer().getRemoteAddress() != null
                ? event.getPlayer().getRemoteAddress().getAddress().getHostAddress()
                : null;
        boolean offline = !plugin.getServer().getConfiguration().isOnlineMode();

        ConnectResponse resp;
        try {
            resp = backendClient.sessionConnect(uuid, username, ip, offline)
                    .get(5, TimeUnit.SECONDS);
        } catch (Exception e) {
            plugin.getLogger().warn("[BetterModeration] sessionConnect timed out for {}: {}", uuid, e.getMessage());
            return; // fail-open
        }

        if (resp == null) return; // fail-open on error

        if (resp.getAction() == ConnectResponse.Action.DENY) {
            String msg = resp.getKickMessage() != null
                    ? resp.getKickMessage()
                    : "§cYou are banned from this network.\n§7Appeal at: §bhttps://bettermoderation.dev/appeal";
            event.setResult(ResultedEvent.ComponentResult.denied(Component.text(msg)));
        }
        // FLAG: backend handles Discord notification — plugin does nothing extra
    }
}
