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

func CreateAccount(name, password string) error {
	conn, err := connect()
	if err != nil {
		return err
	}

	_, err = conn.Exec(context.Background(), "INSERT INTO accounts VALUES ( ?, ? )", name, password)

	// TODO handle and cleanup unqiue violations

	return err
}

func ValidateCredentials(name, password string) error {
	a, err := GetAccount(name)
	if err != nil {
		return err
	}

	// TODO hashing lol

	if a.Password != password {
		return errors.New("invalid credentials")
	}

	return nil
}

type Account struct {
	Name     string
	Password string
}

func GetAccount(name string) (*Account, error) {
	conn, err := connect()
	if err != nil {
		return nil, err
	}

	row := conn.QueryRow(context.Background(), "SELECT name,password FROM accounts WHERE name = ?", name)

	a := &Account{}

	err = row.Scan(&a.Name, &a.Password)
	if err != nil {
		return nil, err
	}

	return a, nil
}

func StartSession(name string) (sessionID string, err error) {
	var conn *pgxpool.Pool
	conn, err = connect()
	if err != nil {
		return
	}

	sessionID = uuid.New().String()

	_, err = conn.Exec(context.Background(), "INSERT INTO sessions VALUES ( ?, ? )", name, sessionID)

	return
}
