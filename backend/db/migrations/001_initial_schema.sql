-- BetterModeration — Initial Schema
-- Engine: InnoDB (crash-safe, ACID, FK support)
-- Charset: utf8mb4 (full Unicode including emoji)
-- All timestamps stored as DATETIME(3) (millisecond precision) in UTC

SET NAMES utf8mb4;
SET time_zone = '+00:00';
SET foreign_key_checks = 0;

-- ─────────────────────────────────────────────
-- ORGANIZATIONS
-- One per community. Everything hangs off this.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS organizations (
    id                  CHAR(36)        NOT NULL,
    name                VARCHAR(64)     NOT NULL,
    owner_discord_id    VARCHAR(32)     NOT NULL,
    public_lookup       TINYINT(1)      NOT NULL DEFAULT 1,
    created_at          DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- SERVERS
-- A connected Discord guild or Minecraft server.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS servers (
    id              CHAR(36)        NOT NULL,
    org_id          CHAR(36)        NOT NULL,
    name            VARCHAR(128)    NOT NULL,
    platform        ENUM('DISCORD','PAPER','VELOCITY','FABRIC') NOT NULL,
    platform_id     VARCHAR(64)     NOT NULL,   -- Discord guild ID or Minecraft server UUID
    api_key_hash    VARCHAR(128)    NOT NULL,   -- bcrypt hash of the server's API key
    online          TINYINT(1)      NOT NULL DEFAULT 0,
    last_seen       DATETIME(3)     NULL,
    linked_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    CONSTRAINT fk_servers_org FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE,
    INDEX idx_servers_org       (org_id),
    INDEX idx_servers_platform  (platform, platform_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- PROFILES
-- One per real-world person. Links Discord + Minecraft.
-- Global — not scoped to an org.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS profiles (
    id              CHAR(36)        NOT NULL,
    discord_id      VARCHAR(32)     NULL UNIQUE,
    minecraft_uuid  VARCHAR(36)     NULL UNIQUE,
    linked_at       DATETIME(3)     NULL,       -- when Discord + Minecraft were linked
    created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_profiles_discord   (discord_id),
    INDEX idx_profiles_minecraft (minecraft_uuid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- PROFILE IPS
-- IP history per profile for alt detection.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS profile_ips (
    profile_id  CHAR(36)        NOT NULL,
    ip_address  VARCHAR(45)     NOT NULL,   -- supports IPv6
    first_seen  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    last_seen   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (profile_id, ip_address),
    CONSTRAINT fk_profile_ips_profile FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    INDEX idx_profile_ips_ip (ip_address)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- PUNISHMENTS
-- All moderation actions: ban, mute, kick, warn, note.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS punishments (
    id                  CHAR(36)        NOT NULL,
    org_id              CHAR(36)        NOT NULL,
    profile_id          CHAR(36)        NOT NULL,
    type                ENUM('BAN','MUTE','KICK','WARN','NOTE') NOT NULL,
    reason              TEXT            NOT NULL,
    issued_by           CHAR(36)        NULL,       -- profile_id of the mod; NULL = system
    issued_by_type      ENUM('STAFF','SYSTEM','AUTOMOD','API') NOT NULL DEFAULT 'STAFF',
    issued_at           DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    expires_at          DATETIME(3)     NULL,       -- NULL = permanent
    platform            ENUM('DISCORD','MINECRAFT','SYSTEM') NOT NULL,
    server_id           CHAR(36)        NULL,
    active              TINYINT(1)      NOT NULL DEFAULT 1,
    public              TINYINT(1)      NOT NULL DEFAULT 1,
    revoked_by          CHAR(36)        NULL,
    revoked_at          DATETIME(3)     NULL,
    revoke_reason       VARCHAR(512)    NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_punishments_org       FOREIGN KEY (org_id)     REFERENCES organizations(id) ON DELETE CASCADE,
    CONSTRAINT fk_punishments_profile   FOREIGN KEY (profile_id) REFERENCES profiles(id)      ON DELETE CASCADE,
    CONSTRAINT fk_punishments_server    FOREIGN KEY (server_id)  REFERENCES servers(id)        ON DELETE SET NULL,
    INDEX idx_punishments_profile_active (profile_id, active),
    INDEX idx_punishments_org_type       (org_id, type),
    INDEX idx_punishments_expires        (expires_at, active),   -- for the expiry worker
    INDEX idx_punishments_issued_at      (issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- WARNING THRESHOLDS
-- At N active warnings → auto-issue punishment P.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS warning_thresholds (
    id                  CHAR(36)        NOT NULL,
    org_id              CHAR(36)        NOT NULL,
    warn_count          TINYINT UNSIGNED NOT NULL,  -- trigger at this many active warnings
    action_type         ENUM('BAN','MUTE','KICK')   NOT NULL,
    duration_seconds    BIGINT UNSIGNED NULL,        -- NULL = permanent
    decay_days          SMALLINT UNSIGNED NULL,      -- warnings older than this don't count; NULL = never decay
    PRIMARY KEY (id),
    CONSTRAINT fk_thresholds_org FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE,
    UNIQUE KEY uq_threshold_org_count (org_id, warn_count),
    INDEX idx_thresholds_org (org_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- MOD ROLES
-- Tiered moderator roles per org.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS mod_roles (
    id                          CHAR(36)        NOT NULL,
    org_id                      CHAR(36)        NOT NULL,
    name                        VARCHAR(64)     NOT NULL,
    discord_role_id             VARCHAR(32)     NULL,
    minecraft_permission_node   VARCHAR(128)    NULL,
    priority                    TINYINT UNSIGNED NOT NULL DEFAULT 0,  -- higher = more powerful
    PRIMARY KEY (id),
    CONSTRAINT fk_mod_roles_org FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE,
    INDEX idx_mod_roles_org (org_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- MOD ROLE PERMISSIONS
-- Per-role caps on each action type.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS mod_role_permissions (
    id                      CHAR(36)        NOT NULL,
    role_id                 CHAR(36)        NOT NULL,
    action_type             ENUM('BAN','MUTE','KICK','WARN','NOTE') NOT NULL,
    max_duration_seconds    BIGINT UNSIGNED NULL,   -- NULL = no cap (forever allowed)
    rate_limit_count        SMALLINT UNSIGNED NULL, -- NULL = no rate limit
    rate_limit_window_seconds INT UNSIGNED  NULL,   -- window for the rate limit
    PRIMARY KEY (id),
    CONSTRAINT fk_mrp_role FOREIGN KEY (role_id) REFERENCES mod_roles(id) ON DELETE CASCADE,
    UNIQUE KEY uq_mrp_role_action (role_id, action_type),
    INDEX idx_mrp_role (role_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- MOD ROLE USAGE
-- Tracks how many actions a mod has taken in a window (for rate limiting).
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS mod_role_usage (
    id              CHAR(36)        NOT NULL,
    profile_id      CHAR(36)        NOT NULL,
    role_id         CHAR(36)        NOT NULL,
    action_type     ENUM('BAN','MUTE','KICK','WARN','NOTE') NOT NULL,
    action_count    SMALLINT UNSIGNED NOT NULL DEFAULT 0,
    window_start    DATETIME(3)     NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_mru_profile FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    CONSTRAINT fk_mru_role    FOREIGN KEY (role_id)    REFERENCES mod_roles(id) ON DELETE CASCADE,
    UNIQUE KEY uq_mru_profile_role_action_window (profile_id, role_id, action_type, window_start),
    INDEX idx_mru_window (window_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- APPEALS
-- Players appeal punishments via the website.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS appeals (
    id                  CHAR(36)        NOT NULL,
    punishment_id       CHAR(36)        NOT NULL,
    submitter_id        CHAR(36)        NOT NULL,   -- profile_id of the appealing player
    reason              TEXT            NOT NULL,
    evidence            TEXT            NULL,
    status              ENUM('PENDING','UNDER_REVIEW','APPROVED','DENIED','ESCALATED') NOT NULL DEFAULT 'PENDING',
    assigned_to         CHAR(36)        NULL,
    reviewer_note       TEXT            NULL,
    submitted_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at          DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    resolved_at         DATETIME(3)     NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_appeals_punishment FOREIGN KEY (punishment_id) REFERENCES punishments(id) ON DELETE CASCADE,
    CONSTRAINT fk_appeals_submitter  FOREIGN KEY (submitter_id)  REFERENCES profiles(id)    ON DELETE CASCADE,
    UNIQUE KEY uq_appeals_punishment (punishment_id),   -- one active appeal per punishment
    INDEX idx_appeals_status        (status),
    INDEX idx_appeals_assigned      (assigned_to),
    INDEX idx_appeals_submitted_at  (submitted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- REPORTS
-- Player reports submitted in-game or via Discord.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS reports (
    id                  CHAR(36)        NOT NULL,
    org_id              CHAR(36)        NOT NULL,
    reporter_id         CHAR(36)        NOT NULL,
    target_id           CHAR(36)        NOT NULL,
    reason_category     ENUM('HACKING','HARASSMENT','SPAM','ADVERTISING','TOXICITY','OTHER') NOT NULL,
    description         TEXT            NULL,
    evidence            TEXT            NULL,
    platform            ENUM('DISCORD','MINECRAFT') NOT NULL,
    server_id           CHAR(36)        NULL,
    status              ENUM('OPEN','CLAIMED','RESOLVED','DISMISSED','DUPLICATE','ESCALATED') NOT NULL DEFAULT 'OPEN',
    claimed_by          CHAR(36)        NULL,
    resolution_type     ENUM('ACTION_TAKEN','NO_ACTION','CANNOT_VERIFY','DUPLICATE') NULL,
    punishment_id       CHAR(36)        NULL,
    submitted_at        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at          DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    resolved_at         DATETIME(3)     NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_reports_org        FOREIGN KEY (org_id)       REFERENCES organizations(id) ON DELETE CASCADE,
    CONSTRAINT fk_reports_reporter   FOREIGN KEY (reporter_id)  REFERENCES profiles(id)      ON DELETE CASCADE,
    CONSTRAINT fk_reports_target     FOREIGN KEY (target_id)    REFERENCES profiles(id)      ON DELETE CASCADE,
    CONSTRAINT fk_reports_server     FOREIGN KEY (server_id)    REFERENCES servers(id)        ON DELETE SET NULL,
    CONSTRAINT fk_reports_punishment FOREIGN KEY (punishment_id) REFERENCES punishments(id)  ON DELETE SET NULL,
    INDEX idx_reports_org_status     (org_id, status),
    INDEX idx_reports_target         (target_id),
    INDEX idx_reports_submitted_at   (submitted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- AUTO MOD RULES
-- Configurable auto-moderation rules per org.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS automod_rules (
    id                      CHAR(36)        NOT NULL,
    org_id                  CHAR(36)        NOT NULL,
    name                    VARCHAR(64)     NOT NULL,
    enabled                 TINYINT(1)      NOT NULL DEFAULT 1,
    test_mode               TINYINT(1)      NOT NULL DEFAULT 0,
    trigger_type            ENUM('WORD_MATCH','REGEX','SPAM','MENTION_SPAM','CAPS','LINK','INVITE') NOT NULL,
    trigger_value           TEXT            NOT NULL,
    action_type             ENUM('DELETE','WARN','MUTE','KICK','BAN','NOTIFY_STAFF') NOT NULL,
    action_duration_seconds BIGINT UNSIGNED NULL,
    platform                ENUM('DISCORD','MINECRAFT','ALL') NOT NULL DEFAULT 'ALL',
    priority                SMALLINT UNSIGNED NOT NULL DEFAULT 0,
    created_at              DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    CONSTRAINT fk_automod_org FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE,
    INDEX idx_automod_org_enabled (org_id, enabled),
    INDEX idx_automod_priority    (priority)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- AUDIT LOG
-- Immutable append-only record of all actions.
-- No UPDATE or DELETE should ever touch this table.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS audit_log (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    org_id          CHAR(36)        NOT NULL,
    actor_id        CHAR(36)        NULL,   -- profile_id; NULL = system
    actor_type      ENUM('STAFF','SYSTEM','AUTOMOD','API') NOT NULL DEFAULT 'STAFF',
    action          VARCHAR(64)     NOT NULL,  -- e.g. PUNISHMENT_ISSUED, APPEAL_APPROVED
    target_id       CHAR(36)        NULL,
    punishment_id   CHAR(36)        NULL,
    details         JSON            NULL,
    ip_address      VARCHAR(45)     NULL,
    source          ENUM('COMMAND','DASHBOARD','API','AUTOMOD','DISCORD_BOT','MINECRAFT_PLUGIN') NOT NULL,
    created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    INDEX idx_audit_org         (org_id),
    INDEX idx_audit_actor       (actor_id),
    INDEX idx_audit_target      (target_id),
    INDEX idx_audit_action      (action),
    INDEX idx_audit_created_at  (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- LINK CODES
-- Temporary one-time codes for server/player linking.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS link_codes (
    code        VARCHAR(16)     NOT NULL,
    type        ENUM('SERVER_LINK','PLAYER_LINK') NOT NULL,
    org_id      CHAR(36)        NULL,
    profile_id  CHAR(36)        NULL,
    platform_id VARCHAR(64)     NOT NULL,   -- Discord guild ID or Minecraft UUID
    expires_at  DATETIME(3)     NOT NULL,
    used        TINYINT(1)      NOT NULL DEFAULT 0,
    PRIMARY KEY (code),
    INDEX idx_link_codes_expires (expires_at, used)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- WEBHOOK SUBSCRIPTIONS
-- Outbound webhooks fired on moderation events.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS webhook_subscriptions (
    id          CHAR(36)        NOT NULL,
    org_id      CHAR(36)        NOT NULL,
    name        VARCHAR(64)     NOT NULL,
    url         VARCHAR(512)    NOT NULL,
    secret      VARCHAR(128)    NOT NULL,   -- HMAC-SHA256 signing secret
    events      JSON            NOT NULL,   -- array of event type strings
    enabled     TINYINT(1)      NOT NULL DEFAULT 1,
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    CONSTRAINT fk_webhooks_org FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE,
    INDEX idx_webhooks_org (org_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- API KEYS
-- Third-party API authentication per org.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS api_keys (
    id          CHAR(36)        NOT NULL,
    org_id      CHAR(36)        NOT NULL,
    name        VARCHAR(64)     NOT NULL,
    key_hash    VARCHAR(128)    NOT NULL,   -- bcrypt hash of the raw key
    permissions JSON            NOT NULL,   -- array of permission scopes
    last_used   DATETIME(3)     NULL,
    expires_at  DATETIME(3)     NULL,
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    CONSTRAINT fk_api_keys_org FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE,
    INDEX idx_api_keys_org  (org_id),
    INDEX idx_api_keys_hash (key_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET foreign_key_checks = 1;
