package db

import (
	"context"
	_ "embed"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
)

// go:embed schema.sql
var schema string

func EnsureSchema() {
	// TODO look into tern
}

func connect() (*pgxpool.Pool, error) {
	// TODO read dburl from environment
	conn, err := pgxpool.Connect(context.Background(), "postgres://vilmibm:vilmibm@localhost:5432/hermeticum")
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func CreateAccount(name, password string) (*Account, error) {
	conn, err := connect()
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

func ValidateCredentials(name, password string) (*Account, error) {
	a, err := GetAccount(name)
	if err != nil {
		return nil, err
	}

	// TODO hashing lol

	if a.Pwhash != password {
		return nil, errors.New("invalid credentials")
	}

	return a, nil
}

type Account struct {
	ID     int
	Name   string
	Pwhash string
}

func GetAccount(name string) (*Account, error) {
	conn, err := connect()
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

func StartSession(a Account) (sessionID string, err error) {
	var conn *pgxpool.Pool
	conn, err = connect()
	if err != nil {
		return
	}

	sessionID = uuid.New().String()

	_, err = conn.Exec(context.Background(), "INSERT INTO sessions (session_id, account) VALUES ( $1, $2 )", sessionID, a.ID)

	return
}
