-- Migration: Add password_hash column to users table
-- Date: 2026-04-22
-- Ticket: e0a4c2d9d8 (Web UI Login System)

-- SQLite migration
ALTER TABLE users ADD COLUMN password_hash TEXT;
