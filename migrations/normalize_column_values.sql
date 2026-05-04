-- Migration to normalize legacy column values
-- Fixes mixed naming conventions in the tickets table

UPDATE tickets SET column = 'todo-0' WHERE column = 'todo';
UPDATE tickets SET column = 'done-0' WHERE column = 'done';

-- Note: Cancelled-0 remains as-is (Title Case) for now - see ticket if normalization needed
