-- Migration: Add password_hash to users table for web UI authentication
-- Date: 2026-04-22
-- Ticket: ticket-e0a4c2d9d8 - Implement web UI login system with session persistence

ALTER TABLE users ADD COLUMN password_hash TEXT;

-- Update existing users with a default password hash (they'll need to set passwords via admin tools)
-- Using bcrypt hash for 'changeme' as placeholder
UPDATE users SET password_hash = '$2a$10$defaultHashPlaceholderChangeViaAdmin' WHERE password_hash IS NULL;
