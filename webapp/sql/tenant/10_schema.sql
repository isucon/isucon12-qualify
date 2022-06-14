DROP TABLE IF EXISTS competition;
DROP TABLE IF EXISTS player;
DROP TABLE IF EXISTS player_score;

CREATE TABLE competition (
  id TEXT NOT NULL PRIMARY KEY,
  title TEXT NOT NULL,
  finished_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE player (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  is_disqualified INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE player_score (
  player_id TEXT NOT NULL,
  competition_id TEXT NOT NULL,
  score INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (player_id, competition_id)
);
