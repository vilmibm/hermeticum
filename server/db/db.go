package db

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"math/rand"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
)

// go:embed schema.sql
var schema string

// TODO I have a suspicion that I'm going to want to move to an ORM like model where the object struct has a DB member and various methods on it. For now I'm just going to keep adding shit to the DB interface because if it doesn't get too long in the end then it's fine.

type DB interface {
	// Accounts
	CreateAccount(string, string) (*Account, error)
	ValidateCredentials(string, string) (*Account, error)
	GetAccount(string) (*Account, error)
	StartSession(Account) (string, error)
	EndSession(string) error

	// Presence
	AvatarBySessionID(string) (*Object, error)
	BedroomBySessionID(string) (*Object, error)
	MoveInto(toMove Object, container Object) error
}

type Account struct {
	ID     int
	Name   string
	Pwhash string
}

type Object struct {
	ID      int
	Avatar  bool
	Bedroom bool
	Data    map[string]string
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

func (db *pgDB) CreateAccount(name, password string) (account *Account, err error) {
	ctx := context.Background()
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return
	}

	defer tx.Rollback(ctx)

	account = &Account{
		Name:   name,
		Pwhash: password,
	}

	stmt := "INSERT INTO accounts (name, pwhash) VALUES ( $1, $2 ) RETURNING id"
	err = tx.QueryRow(ctx, stmt, name, password).Scan(&account.ID)
	// TODO handle and cleanup unqiue violations
	if err != nil {
		return
	}

	data := map[string]string{}
	data["name"] = account.Name
	data["description"] = fmt.Sprintf("a gaseous form. it smells faintly of %s.", randSmell())
	av := &Object{
		Avatar: true,
		Data:   data,
	}

	stmt = "INSERT INTO objects ( avatar, data, owner ) VALUES ( $1, $2, $3 ) RETURNING id"
	err = tx.QueryRow(ctx, stmt, av.Avatar, av.Data, account.ID).Scan(&av.ID)
	if err != nil {
		return
	}

	stmt = "INSERT INTO permissions (object) VALUES ( $1 )"
	_, err = tx.Exec(ctx, stmt, av.ID)
	if err != nil {
		return
	}

	data = map[string]string{}
	data["name"] = "your private bedroom"

	bedroom := &Object{
		Bedroom: true,
		Data:    data,
	}

	stmt = "INSERT INTO objects ( bedroom, data, owner ) VALUES ( $1, $2, $3 ) RETURNING id"
	err = tx.QueryRow(ctx, stmt, bedroom.Bedroom, bedroom.Data, account.ID).Scan(&bedroom.ID)
	if err != nil {
		return
	}

	stmt = "INSERT INTO permissions (object) VALUES ( $1 )"
	_, err = tx.Exec(ctx, stmt, bedroom.ID)
	if err != nil {
		return
	}

	err = tx.Commit(ctx)

	return
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

func (db *pgDB) GetAccount(name string) (a *Account, err error) {
	a = &Account{}
	stmt := "SELECT id, name, pwhash FROM accounts WHERE name = $1"
	err = db.pool.QueryRow(context.Background(), stmt, name).Scan(&a.ID, &a.Name, &a.Pwhash)
	return
}

func (db *pgDB) StartSession(a Account) (sid string, err error) {
	sid = uuid.New().String()
	_, err = db.pool.Exec(context.Background(),
		"INSERT INTO sessions (id, account) VALUES ( $1, $2 )", sid, a.ID)
	if err != nil {
		return
	}

	// Clean up any ghosts to prevent avatar duplication
	// TODO subquery
	stmt := "DELETE FROM contains WHERE contained = (SELECT id FROM objects WHERE objects.avatar = true and objects.owner = $1)"
	_, err = db.pool.Exec(context.Background(), stmt, a.ID)
	if err != nil {
		log.Printf("failed to clean up ghosts for %d: %s", a.ID, err.Error())
		err = nil
	}

	return
}

func (db *pgDB) EndSession(sid string) (err error) {
	if sid == "" {
		log.Println("db.EndSession called with empty session id")
		return
	}

	var o *Object
	if o, err = db.AvatarBySessionID(sid); err == nil {
		if _, err = db.pool.Exec(context.Background(),
			"DELETE FROM contains WHERE contained = $1", o.ID); err != nil {
			log.Printf("failed to remove avatar from room: %s", err.Error())
		}
	} else {
		log.Printf("failed to find avatar for session %s: %s", sid, err.Error())
	}

	_, err = db.pool.Exec(context.Background(), "DELETE FROM sessions WHERE id = $1", sid)

	return
}

func (db *pgDB) AvatarBySessionID(sid string) (avatar *Object, err error) {
	avatar = &Object{}

	// TODO subquery
	stmt := `
	SELECT id, avatar, data
	FROM objects WHERE avatar = true AND owner = (
		SELECT a.id FROM sessions s JOIN accounts a ON s.account = a.id WHERE s.id = $1)`
	err = db.pool.QueryRow(context.Background(), stmt, sid).Scan(
		&avatar.ID, &avatar.Avatar, &avatar.Data)
	return
}

func (db *pgDB) BedroomBySessionID(sid string) (bedroom *Object, err error) {
	bedroom = &Object{}

	// TODO subquery
	stmt := `
	SELECT id, bedroom, data
	FROM objects WHERE bedroom = true AND owner = (
		SELECT a.id FROM sessions s JOIN accounts a ON s.account = a.id WHERE s.id = $1)`
	err = db.pool.QueryRow(context.Background(), stmt, sid).Scan(
		&bedroom.ID, &bedroom.Bedroom, &bedroom.Data)
	return
}

func (db *pgDB) MoveInto(toMove Object, container Object) error {
	ctx := context.Background()
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	stmt := "DELETE FROM contains WHERE contained = $1"
	_, err = tx.Exec(ctx, stmt, toMove.ID)
	if err != nil {
		return err
	}

	stmt = "INSERT INTO contains (contained, container) VALUES ($1, $2)"
	_, err = tx.Exec(ctx, stmt, toMove.ID, container.ID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
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
