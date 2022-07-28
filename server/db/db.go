package db

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"

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
	CreateAvatar(*Account) (*Object, error)
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

	_, err = conn.Exec(context.Background(), "INSERT INTO sessions (id, account) VALUES ( $1, $2 )", sessionID, a.ID)

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

	_, err = conn.Exec(context.Background(), "DELETE FROM sessions WHERE id = $1", sid)

	return err
}

type Object struct {
	ID      int
	Avatar  bool
	Bedroom bool
	data    string
}

func (db *pgDB) CreateAvatar(account *Account) (avatar *Object, err error) {
	// TODO start a transaction
	data := map[string]string{}
	data["name"] = account.Name
	data["description"] = fmt.Sprintf("a gaseous form. it smells faintly of %s.", randSmell())
	d, _ := json.Marshal(data)
	avatar = &Object{
		Avatar: true,
		data:   string(d),
	}

	// TODO I need to understand how to make use of INSERT...RETURNING

	// TODO how do I determine what perm id to use? I might want to revisit the
	// schema for that so perm knows about an object and not the other way
	// around. I could also just store this data on the objects table. I will
	// ponder if there is any reasonable argument for a separate permissions
	// table.

	_, err = db.pool.Exec(context.Background(), "INSERT ")
	if err != nil {
		return nil, err
	}

	// TODO fetch and return avatar

	return
}

func randSmell() string {
	// TODO seeding
	smells := []string{
		"lavender",
		"wet soil",
		"juniper",
		"pine sap",
		"wood smoke",
	}
	ix := rand.Intn(len(smells))
	return smells[ix]
}
