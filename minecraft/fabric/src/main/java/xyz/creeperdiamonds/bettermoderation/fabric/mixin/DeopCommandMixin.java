package xyz.creeperdiamonds.bettermoderation.fabric.mixin;

import net.minecraft.server.command.DeopCommand;
import net.minecraft.server.command.ServerCommandSource;
import com.mojang.brigadier.context.CommandContext;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfoReturnable;

/**
 * Blocks /deop on offline-mode servers alongside OpCommandMixin — since op is
 * stripped on join anyway, /deop is unnecessary and its presence could confuse admins.
 */
@Mixin(DeopCommand.class)
public abstract class DeopCommandMixin {

    @Inject(method = "deop", at = @At("HEAD"), cancellable = true)
    private static void bettermoderation$blockDeop(CommandContext<ServerCommandSource> ctx, CallbackInfoReturnable<Integer> cir) {
        if (!ctx.getSource().getServer().isOnlineMode()) {
            ctx.getSource().sendError(net.minecraft.text.Text.literal(
                "[BetterModeration] /deop is disabled on offline-mode servers. Use a permissions plugin."
            ));
            cir.setReturnValue(0);
        }
    }
}
