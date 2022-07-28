CREATE TABLE accounts (
	id       serial PRIMARY KEY,
	name     varchar(100)  NOT NULL,
	pwhash   varchar(100)  NOT NULL,
	god      boolean       NOT NULL DEFAULT FALSE
);

CREATE TABLE sessions (
  session_id varchar(100) PRIMARY KEY,
  account integer references accounts ON DELETE CASCADE
);

CREATE TYPE perm AS ENUM ('owner', 'world');

-- owner = 1, world = 2
CREATE TABLE permissions (
  id    serial  PRIMARY KEY,
  read  perm    NOT NULL DEFAULT 'world',
  write perm    NOT NULL DEFAULT 'owner',
  carry perm    NOT NULL DEFAULT 'world',
  exec  perm    NOT NULL DEFAULT 'owner'
);

CREATE TABLE objects (
  id        serial  PRIMARY KEY,
  shortname varchar(200) NOT NULL UNIQUE,
  avatar    boolean NOT NULL DEFAULT FALSE,
  bedroom   boolean NOT NULL DEFAULT FALSE,
  data      jsonb   NOT NULL,

  perms   integer references permissions,
  owner   integer references accounts
);


CREATE TABLE contains (
  container integer references objects ON DELETE RESTRICT,
  contained integer references objects ON DELETE CASCADE
);
