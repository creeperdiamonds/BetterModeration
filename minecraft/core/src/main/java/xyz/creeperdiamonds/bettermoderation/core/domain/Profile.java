package xyz.creeperdiamonds.bettermoderation.core.domain;

import java.util.List;

/**
 * Represents a cross-platform player profile linking a Discord account to a Minecraft account.
 */
public class Profile {

    private final String id;
    private final String discordId;
    private final String minecraftUuid;
    private final long linkedAt;
    private final List<String> ipHistory;

    public Profile(String id, String discordId, String minecraftUuid, long linkedAt, List<String> ipHistory) {
        this.id = id;
        this.discordId = discordId;
        this.minecraftUuid = minecraftUuid;
        this.linkedAt = linkedAt;
        this.ipHistory = ipHistory;
    }

    public String getId() { return id; }
    public String getDiscordId() { return discordId; }
    public String getMinecraftUuid() { return minecraftUuid; }
    public long getLinkedAt() { return linkedAt; }
    public List<String> getIpHistory() { return ipHistory; }
}
