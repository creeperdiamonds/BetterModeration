package xyz.creeperdiamonds.bettermoderation.fabric.mixin;

import xyz.creeperdiamonds.bettermoderation.fabric.BetterModerationFabric;
import net.minecraft.server.command.OpCommand;
import net.minecraft.server.command.ServerCommandSource;
import com.mojang.brigadier.context.CommandContext;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfoReturnable;

/**
 * Blocks /op and /deop on offline-mode servers. Op stored by username is a
 * security hole on cracked servers — use a permissions plugin instead.
 */
@Mixin(OpCommand.class)
public abstract class OpCommandMixin {

    @Inject(method = "op", at = @At("HEAD"), cancellable = true)
    private static void bettermoderation$blockOp(CommandContext<ServerCommandSource> ctx, CallbackInfoReturnable<Integer> cir) {
        if (!ctx.getSource().getServer().isOnlineMode()) {
            ctx.getSource().sendError(net.minecraft.text.Text.literal(
                "[BetterModeration] /op is disabled on offline-mode servers. Use a permissions plugin."
            ));
            cir.setReturnValue(0);
        }
    }
}
