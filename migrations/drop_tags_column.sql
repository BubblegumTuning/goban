-- Migration to remove deprecated tags column
-- Run this on existing databases before upgrading

ALTER TABLE tickets DROP COLUMN IF EXISTS tags;
