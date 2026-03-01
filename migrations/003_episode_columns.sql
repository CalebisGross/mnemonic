-- Migration 003: Add missing episode columns
-- The episodes table was created without concepts, files_modified, and
-- event_timeline columns that the application code expects.

ALTER TABLE episodes ADD COLUMN concepts TEXT;
ALTER TABLE episodes ADD COLUMN files_modified TEXT;
ALTER TABLE episodes ADD COLUMN event_timeline TEXT;
