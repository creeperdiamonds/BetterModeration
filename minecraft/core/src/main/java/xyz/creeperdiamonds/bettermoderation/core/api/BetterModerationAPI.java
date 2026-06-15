package xyz.creeperdiamonds.bettermoderation.core.api;

import xyz.creeperdiamonds.bettermoderation.core.domain.Profile;
import xyz.creeperdiamonds.bettermoderation.core.domain.Punishment;
import xyz.creeperdiamonds.bettermoderation.core.domain.PunishmentType;

import java.util.List;

/**
 * Core API contract for issuing and querying moderation actions.
 * Implementations are provided by each platform adapter (Paper, Velocity, Fabric).
 */
public interface BetterModerationAPI {

    /**
     * Issues a new punishment against the given profile.
     *
     * @param profileId  the internal BetterModeration profile ID
     * @param type       the punishment type (BAN, MUTE, KICK, WARN, NOTE)
     * @param reason     human-readable reason for the punishment
     * @param issuedBy   identifier of the staff member or system issuing the punishment
     * @param expiresAt  epoch millisecond timestamp when the punishment expires, or null for permanent
     * @return the newly created {@link Punishment}
     */
    Punishment issuePunishment(String profileId, PunishmentType type, String reason, String issuedBy, Long expiresAt);

    /**
     * Revokes (deactivates) an existing punishment by its ID.
     *
     * @param punishmentId the ID of the punishment to revoke
     * @param revokedBy    identifier of the staff member revoking the punishment
     */
    void revokePunishment(String punishmentId, String revokedBy);

    /**
     * Returns all currently active (non-expired, non-revoked) punishments for a profile.
     *
     * @param profileId the internal BetterModeration profile ID
     * @return list of active punishments, may be empty
     */
    List<Punishment> getActivePunishments(String profileId);

    /**
     * Looks up the cross-platform profile associated with a Minecraft UUID.
     *
     * @param minecraftUuid the player's Minecraft UUID (with or without dashes)
     * @return the matching {@link Profile}, or null if not linked
     */
    Profile getProfile(String minecraftUuid);
}
