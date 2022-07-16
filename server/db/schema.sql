CREATE TABLE accounts (
	id       serial PRIMARY KEY,
	name     varchar(100)  NOT NULL,
	pwhash   varchar(100)  NOT NULL,
	god      boolean       DEFAULT FALSE
);

CREATE TABLE sessions (
  session_id varchar(100) PRIMARY KEY,
  account integer references accounts(id)
);
