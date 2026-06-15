package xyz.creeperdiamonds.bettermoderation.core.domain;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.time.Instant;

/**
 * Represents a moderation action issued against a player profile.
 * Designed to deserialize from the BetterModeration backend JSON API,
 * which uses snake_case field names and RFC 3339 date strings.
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public class Punishment {

    @JsonProperty("id")
    private String id;

    @JsonProperty("profile_id")
    private String profileId;

    // Stored as string to avoid requiring JavaTimeModule for ISO 8601 parsing.
    @JsonProperty("issued_at")
    private String issuedAt;

    // Null means permanent ban.
    @JsonProperty("expires_at")
    private String expiresAt;

    @JsonProperty("type")
    private String typeRaw;

    @JsonProperty("reason")
    private String reason;

    @JsonProperty("issued_by")
    private String issuedBy;

    @JsonProperty("platform")
    private String platform;

    @JsonProperty("server_id")
    private String serverId;

    // The backend field is minecraft_active (bool). When false the punishment
    // has been revoked and should be ignored by plugins.
    @JsonProperty("minecraft_active")
    private boolean active;

    // Default no-arg constructor required by Jackson.
    public Punishment() {}

    public boolean isExpired() {
        if (expiresAt == null) return false;
        try {
            return Instant.parse(expiresAt).isBefore(Instant.now());
        } catch (Exception e) {
            return false;
        }
    }

    public String getId() { return id; }
    public String getProfileId() { return profileId; }

    public PunishmentType getType() {
        try {
            return PunishmentType.valueOf(typeRaw);
        } catch (Exception e) {
            return null;
        }
    }

    public String getReason() { return reason; }
    public String getIssuedBy() { return issuedBy; }

    /** Epoch millis of issuedAt, or 0 if unparseable. */
    public long getIssuedAt() {
        if (issuedAt == null) return 0L;
        try { return Instant.parse(issuedAt).toEpochMilli(); } catch (Exception e) { return 0L; }
    }

    /**
     * Epoch millis of expiresAt, or null if permanent.
     * Kept as Long for backward compat with existing kick-message formatting.
     */
    public Long getExpiresAt() {
        if (expiresAt == null) return null;
        try { return Instant.parse(expiresAt).toEpochMilli(); } catch (Exception e) { return null; }
    }

    public String getPlatform() { return platform; }
    public String getServerId() { return serverId; }
    public boolean isActive() { return active; }
}
