-- Migration 002: Remove DB state that Discord already manages natively.
--
-- Discord is the source of truth for:
--   - Ban list (query GuildBans)
--   - Timeout state (member.CommunicationDisabledUntil)
--   - Channel permission overwrites (lock/unlock)
--   - Message deletion (purge)
--
-- Our DB only needs to track:
--   - Warns and notes (no Discord equivalent)
--   - Minecraft-side ban/mute state (active + expires_at)
--   - Cross-platform punishment records for history and sync
--
-- Change: `active` is renamed to `minecraft_active` to make clear it only
-- drives Minecraft enforcement. Discord active state is always read from Discord's API.
-- For Discord-side punishments we still keep the record for history, but never
-- use our DB to decide "is this person banned on Discord" — we ask Discord.

ALTER TABLE punishments
    CHANGE COLUMN active minecraft_active TINYINT(1) NOT NULL DEFAULT 0
        COMMENT 'Drives Minecraft enforcement only. Discord ban/timeout state is read from Discord API.';

-- Temp-ban expiry is still stored here so the background worker knows when to call
-- GuildBanDelete. Discord has no native temporary ban support.
-- expires_at remains unchanged.

-- Revoke fields are still useful for Minecraft (to know when a ban was lifted)
-- and for our own audit history. No change there.
