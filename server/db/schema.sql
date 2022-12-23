CREATE TABLE accounts (
	id       serial PRIMARY KEY,
	name     varchar(100)  NOT NULL UNIQUE,
	pwhash   varchar(100)  NOT NULL,
	god      boolean       NOT NULL DEFAULT FALSE
);

CREATE TABLE sessions (
  id varchar(100) PRIMARY KEY,
  account integer REFERENCES accounts ON DELETE CASCADE
);

CREATE TYPE perm AS ENUM ('owner', 'world');

CREATE TABLE objects (
  id        serial  PRIMARY KEY,
  avatar    boolean NOT NULL DEFAULT FALSE,
  bedroom   boolean NOT NULL DEFAULT FALSE,
  data      jsonb   NOT NULL,
  script    text    NOT NULL,

  owner   integer REFERENCES accounts ON DELETE RESTRICT
);

-- owner = 1, world = 2
CREATE TABLE permissions (
  id    serial  PRIMARY KEY,
  read  perm    NOT NULL DEFAULT 'world',
  write perm    NOT NULL DEFAULT 'owner',
  carry perm    NOT NULL DEFAULT 'world',
  exec  perm    NOT NULL DEFAULT 'world',

  object integer REFERENCES objects ON DELETE CASCADE
);

CREATE TABLE contains (
  container integer REFERENCES objects ON DELETE RESTRICT,
  contained integer REFERENCES objects ON DELETE CASCADE
);
