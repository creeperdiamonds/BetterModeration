package xyz.creeperdiamonds.bettermoderation.core.sync;

public class BackendEvent {
    private String type;
    private String payload;
    private String serverId;
    private long timestamp;

    public BackendEvent(String type, String payload, String serverId) {
        this.type = type;
        this.payload = payload;
        this.serverId = serverId;
        this.timestamp = System.currentTimeMillis();
    }

    // getters
    public String getType() { return type; }
    public String getPayload() { return payload; }
    public String getServerId() { return serverId; }
    public long getTimestamp() { return timestamp; }
}
