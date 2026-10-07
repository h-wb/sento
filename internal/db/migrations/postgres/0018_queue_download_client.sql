-- A download belongs to the client it was sent to. Empty = sent before
-- this was recorded: such a job is looked up in every client.
ALTER TABLE queue_items ADD COLUMN download_client TEXT NOT NULL DEFAULT '';
