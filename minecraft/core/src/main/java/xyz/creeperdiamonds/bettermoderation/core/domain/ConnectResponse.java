package xyz.creeperdiamonds.bettermoderation.core.domain;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public class ConnectResponse {

    public enum Action { ALLOW, FLAG, DENY }

    @JsonProperty("action")
    private String actionRaw;

    @JsonProperty("kick_message")
    private String kickMessage;

    @JsonProperty("suspicion_score")
    private int suspicionScore;

    @JsonProperty("flags")
    private List<String> flags;

    @JsonProperty("profile_id")
    private String profileId;

    public ConnectResponse() {}

    public Action getAction() {
        if (actionRaw == null) return Action.ALLOW;
        try {
            return Action.valueOf(actionRaw);
        } catch (IllegalArgumentException e) {
            return Action.ALLOW;
        }
    }

    public String getKickMessage() { return kickMessage; }
    public int getSuspicionScore() { return suspicionScore; }
    public List<String> getFlags() { return flags; }
    public String getProfileId() { return profileId; }
}
