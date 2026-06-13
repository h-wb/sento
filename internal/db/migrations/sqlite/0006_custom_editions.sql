-- Custom editions: user-created collections (language variants of a
-- MangaBaka edition, or fully hand-made ones). Metadata refresh adopts a
-- custom row when MangaBaka later lists the same identity tuple
-- (edition, language, publisher, medium) — clearing the flag.
ALTER TABLE collections ADD COLUMN custom INTEGER NOT NULL DEFAULT 0;
