-- Add 'private' value to show_status enum
-- Private shows are only visible to the user who created them

ALTER TYPE show_status ADD VALUE 'private';
