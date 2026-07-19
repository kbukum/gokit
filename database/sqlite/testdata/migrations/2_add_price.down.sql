CREATE TABLE widgets_new (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO widgets_new (id, name) SELECT id, name FROM widgets;
DROP TABLE widgets;
ALTER TABLE widgets_new RENAME TO widgets;
