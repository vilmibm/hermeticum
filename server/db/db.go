package db

import (
	"context"
	_ "embed"
	"errors"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
)

// go:embed schema.sql
var schema string

type Account struct {
	ID     int
	Name   string
	Pwhash string
}

type DB interface {
	// EnsureSchema() TODO look into tern
	CreateAccount(string, string) (*Account, error)
	ValidateCredentials(string, string) (*Account, error)
	GetAccount(string) (*Account, error)
	StartSession(Account) (string, error)
	EndSession(string) error
}

type pgDB struct {
	pool *pgxpool.Pool
}

func NewDB(connURL string) (DB, error) {
	pool, err := pgxpool.Connect(context.Background(), connURL)
	if err != nil {
		return nil, err
	}
	pgdb := &pgDB{
		pool: pool,
	}

	return pgdb, nil
}

func (db *pgDB) CreateAccount(name, password string) (*Account, error) {
	conn, err := db.pool.Acquire(context.Background())
	if err != nil {
		return nil, err
	}

	_, err = conn.Exec(context.Background(),
		"INSERT INTO accounts (name, pwhash) VALUES ( $1, $2 )", name, password)
	if err != nil {
		return nil, err
	}

	row := conn.QueryRow(context.Background(), "SELECT id,name,pwhash FROM accounts WHERE name = $1", name)
	a := &Account{}
	err = row.Scan(&a.ID, &a.Name, &a.Pwhash)
	if err != nil {
		return nil, err
	}

	// TODO handle and cleanup unqiue violations

	return a, err
}

func (db *pgDB) ValidateCredentials(name, password string) (*Account, error) {
	a, err := db.GetAccount(name)
	if err != nil {
		return nil, err
	}

	// TODO hashing lol

	if a.Pwhash != password {
		return nil, errors.New("invalid credentials")
	}

	return a, nil
}

func (db *pgDB) GetAccount(name string) (*Account, error) {
	conn, err := db.pool.Acquire(context.Background())
	if err != nil {
		return nil, err
	}

	row := conn.QueryRow(context.Background(), "SELECT id, name, pwhash FROM accounts WHERE name = $1", name)

	a := &Account{}

	err = row.Scan(&a.ID, &a.Name, &a.Pwhash)
	if err != nil {
		return nil, err
	}

	return a, nil
}

func (db *pgDB) StartSession(a Account) (sessionID string, err error) {
	conn, err := db.pool.Acquire(context.Background())
	if err != nil {
		return "", err
	}

	sessionID = uuid.New().String()

	_, err = conn.Exec(context.Background(), "INSERT INTO sessions (session_id, account) VALUES ( $1, $2 )", sessionID, a.ID)

	return
}

func (db *pgDB) EndSession(sid string) error {
	if sid == "" {
		log.Println("db.EndSession called with empty session id")
		return nil
	}

	conn, err := db.pool.Acquire(context.Background())
	if err != nil {
		return err
	}

	_, err = conn.Exec(context.Background(), "DELETE FROM sessions WHERE id = ?", sid)

	return err
}
