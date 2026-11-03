-- Remember explicit review choices even when an archive supplies no book.
-- A trash choice is durable intent; the normal trash journal records the move.
ALTER TABLE files ADD COLUMN review_action TEXT NOT NULL DEFAULT '';
