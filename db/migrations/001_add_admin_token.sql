-- Migration: Add admin_token field to rooms table
-- Date: 2025-11-01
-- Description: Add admin_token field for TShock server admin setup token

-- Add admin_token column if it doesn't exist
ALTER TABLE rooms ADD COLUMN admin_token TEXT;

