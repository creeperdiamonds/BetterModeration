-- Migration 004: Offline/cracked mode ban evasion prevention
-- Adds username tracking, IP enrichment cache, join event log,
-- and pre-computed offline UUIDs for instant lookup on player join.

SET NAMES utf8mb4;
SET time_zone = '+00:00';
SET foreign_key_checks = 0;

-- ─────────────────────────────────────────────
-- PLAYER_USERNAMES
-- Every username ever seen for a profile.
-- Used for username-change detection, Levenshtein comparison,
-- and offline UUID pre-computation on ban issuance.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS player_usernames (
    profile_id  CHAR(36)     NOT NULL,
    username    VARCHAR(16)  NOT NULL,
    first_seen  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    last_seen   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                    ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (profile_id, username),
    CONSTRAINT fk_pu_profile FOREIGN KEY (profile_id)
        REFERENCES profiles(id) ON DELETE CASCADE,
    INDEX idx_pu_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- IP_METADATA
-- Enrichment data from ip-api.com, cached for 7 days per IP.
-- No FK — IPs are global across orgs.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS ip_metadata (
    ip_address   VARCHAR(45)  NOT NULL,
    country_code CHAR(2)      NULL,
    isp          VARCHAR(255) NULL,
    org          VARCHAR(255) NULL,
    asn          VARCHAR(16)  NULL,
    is_vpn       TINYINT(1)   NOT NULL DEFAULT 0,
    is_proxy     TINYINT(1)   NOT NULL DEFAULT 0,
    is_tor       TINYINT(1)   NOT NULL DEFAULT 0,
    is_hosting   TINYINT(1)   NOT NULL DEFAULT 0,
    cached_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (ip_address)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- JOIN_EVENTS
-- Append-only join log. Never updated or deleted.
-- Feeds the new-account signal and time-correlation signal in the scorer.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS join_events (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    profile_id   CHAR(36)     NOT NULL,
    org_id       CHAR(36)     NOT NULL,
    server_id    CHAR(36)     NOT NULL,
    username     VARCHAR(16)  NOT NULL,
    ip_address   VARCHAR(45)  NOT NULL,
    offline_mode TINYINT(1)   NOT NULL DEFAULT 0,
    joined_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    CONSTRAINT fk_je_profile FOREIGN KEY (profile_id)
        REFERENCES profiles(id) ON DELETE CASCADE,
    CONSTRAINT fk_je_org FOREIGN KEY (org_id)
        REFERENCES organizations(id) ON DELETE CASCADE,
    INDEX idx_je_profile_joined (profile_id, joined_at),
    INDEX idx_je_ip_joined      (ip_address, joined_at),
    INDEX idx_je_org_joined     (org_id, joined_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────
-- BANNED_OFFLINE_UUIDS
-- Pre-computed offline mode UUIDs for every known username of every banned profile.
-- Indexed on (offline_uuid, org_id) for O(1) lookup at join time.
-- Populated by the ban hook and the one-time backfill endpoint.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS banned_offline_uuids (
    offline_uuid CHAR(36)     NOT NULL,
    profile_id   CHAR(36)     NOT NULL,
    org_id       CHAR(36)     NOT NULL,
    username     VARCHAR(16)  NOT NULL,
    computed_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (offline_uuid, org_id),
    CONSTRAINT fk_bou_profile FOREIGN KEY (profile_id)
        REFERENCES profiles(id) ON DELETE CASCADE,
    CONSTRAINT fk_bou_org FOREIGN KEY (org_id)
        REFERENCES organizations(id) ON DELETE CASCADE,
    INDEX idx_bou_profile (profile_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET foreign_key_checks = 1;
