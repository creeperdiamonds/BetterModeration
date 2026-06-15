package xyz.creeperdiamonds.bettermoderation.fabric.mixin;

import xyz.creeperdiamonds.bettermoderation.core.domain.Punishment;
import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;
import xyz.creeperdiamonds.bettermoderation.fabric.BetterModerationFabric;
import net.minecraft.network.message.SignedMessage;
import net.minecraft.server.network.ServerGamePacketListenerImpl;
import net.minecraft.server.network.ServerPlayerEntity;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.Shadow;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

import java.time.Instant;
import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.List;
import java.util.concurrent.TimeUnit;

/**
 * Intercepts incoming chat messages to enforce active mutes.
 *
 * Targets {@code ServerGamePacketListenerImpl#handleChat} — the server-side
 * handler for signed chat messages in 1.21.x.
 */
@Mixin(ServerGamePacketListenerImpl.class)
public abstract class ChatMixin {

    private static final DateTimeFormatter DATE_FORMAT =
            DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm 'UTC'").withZone(ZoneId.of("UTC"));

    @Shadow
    public ServerPlayerEntity player;

    @Inject(
            method = "handleChat",
            at = @At("HEAD"),
            cancellable = true
    )
    private void bettermoderation$onHandleChat(SignedMessage message, CallbackInfo ci) {
        if (BetterModerationFabric.getInstance() == null) return;

        String uuid = player.getUuidAsString();

        List<Punishment> punishments;
        try {
            punishments = BetterModerationFabric.getInstance()
                    .getBackendClient()
                    .getActivePunishments(uuid, null)
                    .get(3, TimeUnit.SECONDS);
        } catch (Exception e) {
            BetterModerationFabric.LOGGER.warn("Could not check mute for {}: {}", uuid, e.getMessage());
            return;
        }

        if (punishments == null) return;

        for (Punishment punishment : punishments) {
            if (!punishment.isActive() || punishment.isExpired()) continue;

            if (punishment.getType() == PunishmentType.MUTE) {
                ci.cancel();

                String expiry = punishment.getExpiresAt() == null
                        ? "permanent"
                        : DATE_FORMAT.format(Instant.ofEpochMilli(punishment.getExpiresAt()));

                player.sendMessage(
                        net.minecraft.text.Text.literal(
                                "§cYou are muted and cannot send messages.\n"
                                + "§7Reason: §f" + punishment.getReason() + "\n"
                                + "§7Expires: §f" + expiry
                        )
                );
                return;
            }
        }
    }
}
