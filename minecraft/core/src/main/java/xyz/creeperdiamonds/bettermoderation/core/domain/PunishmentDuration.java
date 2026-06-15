package xyz.creeperdiamonds.bettermoderation.core.domain;

public enum PunishmentDuration {
    HOUR(3600000L),
    DAY(86400000L),
    WEEK(604800000L),
    MONTH(2592000000L),
    YEAR(31536000000L),
    FOREVER(-1L);

    private final long millis;

    PunishmentDuration(long millis) {
        this.millis = millis;
    }

    public long toMillis() {
        return millis;
    }

    public boolean isPermanent() {
        return this == FOREVER;
    }
}
