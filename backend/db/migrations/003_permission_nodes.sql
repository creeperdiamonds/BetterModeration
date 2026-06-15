-- Migration 003: Replace action/duration matrix with a LuckPerms-style node system.
-- Drops mod_role_permissions and replaces it with role_permission_nodes.

DROP TABLE IF EXISTS mod_role_permissions;

-- ─────────────────────────────────────────────
-- ROLE PERMISSION NODES
-- Each row grants or denies a single permission node for a mod role.
-- value = 1 → allow, value = 0 → deny (negation, overrides inherited allow)
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS role_permission_nodes (
    id          CHAR(36)        NOT NULL,
    role_id     CHAR(36)        NOT NULL,
    node        VARCHAR(128)    NOT NULL,
    value       TINYINT(1)      NOT NULL DEFAULT 1,
    PRIMARY KEY (id),
    CONSTRAINT fk_rpn_role FOREIGN KEY (role_id) REFERENCES mod_roles(id) ON DELETE CASCADE,
    UNIQUE KEY uq_rpn_role_node (role_id, node),
    INDEX idx_rpn_role (role_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─────────────────────────────────────────────────────────────────
-- PERMISSION NODE REFERENCE (comments only — not stored in DB)
--
-- PUNISHMENT NODES
--   punish.ban              can issue bans
--   punish.ban.permanent    can ban permanently (no expiry)
--   punish.ban.max.1h       max ban duration: 1 hour
--   punish.ban.max.1d       max ban duration: 1 day
--   punish.ban.max.7d       max ban duration: 7 days
--   punish.ban.max.30d      max ban duration: 30 days
--   punish.ban.max.1y       max ban duration: 1 year
--   punish.mute             can issue mutes/timeouts
--   punish.mute.permanent   can mute permanently (Minecraft only; Discord max 28d)
--   punish.mute.max.1h      max mute duration
--   punish.mute.max.1d
--   punish.mute.max.7d
--   punish.mute.max.28d
--   punish.kick             can kick
--   punish.warn             can warn
--   punish.note             can add private staff notes
--   punish.revoke           can revoke/pardon punishments
--   punish.*               wildcard: all punishment actions
--
-- RATE LIMIT NODES  (most specific match wins)
--   ratelimit.ban.5.1d      max 5 bans per day
--   ratelimit.ban.20.1d     max 20 bans per day
--   ratelimit.mute.10.1d    max 10 mutes per day
--   ratelimit.kick.10.1d    max 10 kicks per day
--   ratelimit.warn.50.1d    max 50 warns per day
--
-- REVIEW / MANAGEMENT NODES
--   appeals.view            can view open appeals
--   appeals.review          can approve/deny appeals
--   appeals.assign          can assign appeals to reviewers
--   reports.view            can view open reports
--   reports.claim           can claim and resolve reports
--   history.view            can view punishment history
--   lookup                  can look up user profiles
--   purge                   can bulk delete messages
--   channel.lock            can lock/unlock channels
--   automod.manage          can manage automod rules
--   config.manage           can change server configuration
--   roles.manage            can manage mod roles and nodes
--
-- WILDCARDS
--   *                       all permissions (owner/admin)
--   punish.*               all punishment types
--   appeals.*              all appeal actions
--   reports.*              all report actions
--
-- NEGATION (value = 0)
--   -punish.ban.permanent   cannot ban permanently even if * is granted
--   -ratelimit.ban.20.1d    deny higher rate limit (enforce lower one)
-- ─────────────────────────────────────────────────────────────────
