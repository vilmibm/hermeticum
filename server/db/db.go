package db

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
)

//go:embed schema.sql
var schema string

// TODO I have a suspicion that I'm going to want to move to an ORM like model where the object struct has a DB member and various methods on it. For now I'm just going to keep adding shit to the DB interface because if it doesn't get too long in the end then it's fine.

type DB interface {
	// Accounts
	CreateAccount(string, string) (*Account, error)
	ValidateCredentials(string, string) (*Account, error)
	GetAccount(string) (*Account, error)
	StartSession(Account) (string, error)
	EndSession(string) error
	ActiveSessions() ([]Session, error)
	ClearSessions() error

	// General
	GetObject(owner, name string) (*Object, error)
	GetObjectByID(ID int) (*Object, error)
	SearchObjectsByName(term string) ([]Object, error)

	// Defaults
	Ensure() error
	Erase() error

	// Presence
	SessionIDForAvatar(Object) (string, error)
	SessionIDForObjID(int) (string, error)
	AvatarBySessionID(string) (*Object, error)
	BedroomBySessionID(string) (*Object, error)
	MoveInto(toMove Object, container Object) error
	Earshot(vantage Object) ([]Object, error)
	Resolve(vantage Object, term string) ([]Object, error)
}

type Account struct {
	ID     int
	Name   string
	Pwhash string
	God    bool
}

type Session struct {
	ID        string
	AccountID int
}

type Object struct {
	ID      int
	Avatar  bool
	Bedroom bool
	Data    map[string]string
	OwnerID int
	Script  string
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

// Erase fully destroys the database's contents, dropping all tables.
func (db *pgDB) Erase() (err error) {
	stmts := []string{
		"DROP SCHEMA public CASCADE",
		"CREATE SCHEMA public",
		"GRANT ALL ON SCHEMA public TO postgres",
		"GRANT ALL ON SCHEMA public TO public",
		"COMMENT ON SCHEMA public IS 'standard public schema'",
	}

	for _, stmt := range stmts {
		if _, err = db.pool.Exec(context.Background(), stmt); err != nil {
			return
		}
	}

	return nil
}

// Ensure checks for and then creates default resources if they do not exist (like the Foyer)
func (db *pgDB) Ensure() error {
	// TODO this is sloppy but shrug
	_, err := db.pool.Exec(context.Background(), schema)
	//log.Println(err)
	sysAcc, err := db.GetAccount("system")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		sysAcc, err = db.CreateGod("system", "")
		if err != nil {
			return fmt.Errorf("failed to create system account: %w", err)
		}
	}

	// TODO for some reason, when the seen() callback runs for foyer we're calling the stub Tell instead of the sid-closured Tell. figure out why.

	roomScript := `
		seen(function()
			tellSender(my("description"))
		end)
	`

	foyer, err := db.GetObject("system", "foyer")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		data := map[string]string{}
		data["name"] = "foyer"
		data["description"] = "a big room. the ceiling is painted with constellations."
		foyer = &Object{
			Data:   data,
			Script: roomScript,
		}
		if err = db.CreateObject(sysAcc, foyer); err != nil {
			return err
		}
	}

	egg, err := db.GetObject("system", "floor egg")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		data := map[string]string{}
		data["name"] = "floor egg"
		data["description"] = "it's an egg and it's on the floor."
		egg = &Object{
			Data:   data,
			Script: "",
		}
		if err = db.CreateObject(sysAcc, egg); err != nil {
			return err
		}
	}

	pub, err := db.GetObject("system", "pub")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		data := map[string]string{}
		data["name"] = "pub"
		data["description"] = "a warm pub constructed of hard wood and brass"
		pub = &Object{
			Data:   data,
			Script: roomScript,
		}
		if err = db.CreateObject(sysAcc, pub); err != nil {
			return err
		}
	}

	oakDoor, err := db.GetObject("system", "oak door")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		data := map[string]string{}
		data["name"] = "oak door"
		data["description"] = "a heavy oak door with a brass handle. an ornate sign says PUB."
		oakDoor = &Object{
			Data: data,
			Script: `
				provides("get tetanus", function(args)
					tellSender("you now have tetanus")
				end)
				goes(north, "pub")
			`,
		}
		if err = db.CreateObject(sysAcc, oakDoor); err != nil {
			return err
		}
	}

	sysAva, err := db.GetAccountAvatar(*sysAcc)
	if err != nil {
		return fmt.Errorf("could not find avatar for system account: %w", err)
	}

	db.MoveInto(*sysAva, *foyer)
	db.MoveInto(*egg, *foyer)
	db.MoveInto(*oakDoor, *foyer)

	return nil
}

func (db *pgDB) CreateGod(name, password string) (account *Account, err error) {
	account = &Account{
		Name:   name,
		Pwhash: password,
		God:    true,
	}

	return account, db.createAccount(account)
}

func (db *pgDB) CreateAccount(name, password string) (account *Account, err error) {
	account = &Account{
		Name:   name,
		Pwhash: password,
	}

	return account, db.createAccount(account)
}

func (db *pgDB) createAccount(account *Account) (err error) {
	ctx := context.Background()
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return
	}

	defer tx.Rollback(ctx)

	stmt := "INSERT INTO accounts (name, pwhash, god) VALUES ( $1, $2, $3 ) RETURNING id"
	err = tx.QueryRow(ctx, stmt, account.Name, account.Pwhash, account.God).Scan(&account.ID)
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
		Script: "",
	}

	av.Script = fmt.Sprintf(`%s
hears(".*", function()
	tellMe(msg)
end)

sees(".*", function()
	showMe(msg)
end)
`, hasInvocation(av))

	stmt = "INSERT INTO objects ( avatar, data, owner, script ) VALUES ( $1, $2, $3, $4 ) RETURNING id"
	err = tx.QueryRow(ctx, stmt, av.Avatar, av.Data, account.ID, av.Script).Scan(&av.ID)
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
		Script:  "",
	}

	stmt = "INSERT INTO objects ( bedroom, data, owner, script ) VALUES ( $1, $2, $3, $4 ) RETURNING id"
	err = tx.QueryRow(ctx, stmt, bedroom.Bedroom, bedroom.Data, account.ID, bedroom.Script).Scan(&bedroom.ID)
	if err != nil {
		return
	}

	stmt = "INSERT INTO permissions (object) VALUES ( $1 )"
	_, err = tx.Exec(ctx, stmt, bedroom.ID)
	if err != nil {
		return
	}

	return tx.Commit(ctx)
}

func (db *pgDB) ValidateCredentials(name, password string) (*Account, error) {
	a, err := db.GetAccount(name)
	if err != nil {
		return nil, err
	}

	if a.Pwhash == "" {
		return nil, errors.New("this account cannot be logged into")
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

func (db *pgDB) SessionIDForAvatar(obj Object) (string, error) {
	if !obj.Avatar {
		return "", nil
	}
	ctx := context.Background()
	stmt := `SELECT id FROM sessions WHERE account = $1`
	var sid *string
	var err error
	if err = db.pool.QueryRow(ctx, stmt, obj.OwnerID).Scan(&sid); err != nil {
		return "", err
	}

	if sid == nil {
		return "", nil
	}

	return *sid, nil
}

func (db *pgDB) SessionIDForObjID(id int) (string, error) {
	obj, err := db.GetObjectByID(id)
	if err != nil {
		return "", err
	}

	return db.SessionIDForAvatar(*obj)

}

func (db *pgDB) AvatarBySessionID(sid string) (avatar *Object, err error) {
	avatar = &Object{}
	// TODO subquery
	stmt := `
	SELECT id, avatar, data, owner, script
	FROM objects WHERE avatar = true AND owner = (
		SELECT a.id FROM sessions s JOIN accounts a ON s.account = a.id WHERE s.id = $1)`
	err = db.pool.QueryRow(context.Background(), stmt, sid).Scan(
		&avatar.ID, &avatar.Avatar, &avatar.Data, &avatar.OwnerID, &avatar.Script)
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

func (db *pgDB) Earshot(obj Object) ([]Object, error) {
	stmt := `
	SELECT id, avatar, bedroom, data, owner, script FROM objects
	WHERE id IN (
		SELECT contained FROM contains
		WHERE container = (
				SELECT container FROM contains WHERE contained = $1 LIMIT 1))
		OR id = (SELECT container FROM contains WHERE contained = $1 LIMIT 1)`
	rows, err := db.pool.Query(context.Background(), stmt, obj.ID)
	if err != nil {
		return nil, err
	}

	out := []Object{}

	for rows.Next() {
		heard := Object{}
		if err = rows.Scan(
			&heard.ID, &heard.Avatar,
			&heard.Bedroom, &heard.Data,
			&heard.OwnerID, &heard.Script); err != nil {
			return nil, err
		}
		out = append(out, heard)
	}

	return out, nil
}

func (db *pgDB) GetObjectByID(ID int) (*Object, error) {
	ctx := context.Background()
	obj := &Object{}
	stmt := `
		SELECT id, avatar, data, owner, script
		FROM objects
		WHERE id = $1`
	err := db.pool.QueryRow(ctx, stmt, ID).Scan(
		&obj.ID, &obj.Avatar, &obj.Data, &obj.OwnerID, &obj.Script)
	return obj, err
}

// TODO fix arg
func (db *pgDB) GetObject(owner, name string) (obj *Object, err error) {
	ctx := context.Background()
	obj = &Object{}
	stmt := `
		SELECT id, avatar, data, owner, script
		FROM objects
		WHERE owner = (SELECT id FROM accounts WHERE name=$1) AND data['name'] = $2`
	err = db.pool.QueryRow(ctx, stmt, owner, fmt.Sprintf(`"%s"`, name)).Scan(
		&obj.ID, &obj.Avatar, &obj.Data, &obj.OwnerID, &obj.Script)

	return
}

func (db *pgDB) SearchObjectsByName(term string) ([]Object, error) {
	ctx := context.Background()

	stmt := `
		SELECT id, avatar, data, owner, script
		FROM objects
		WHERE data['name']::varchar LIKE $1 
	`

	rows, err := db.pool.Query(ctx, stmt, "%"+term+"%")
	if err != nil {
		return nil, err
	}

	out := []Object{}
	for rows.Next() {
		o := Object{}
		if err = rows.Scan(
			&o.ID,
			&o.Avatar,
			&o.Data,
			&o.OwnerID,
			&o.Script); err != nil {
			return nil, err
		}
		out = append(out, o)
	}

	return out, nil
}

func (db *pgDB) Resolve(vantage Object, term string) ([]Object, error) {
	stuff, err := db.Earshot(vantage)
	if err != nil {
		return nil, err
	}

	out := []Object{}

	for _, o := range stuff {
		if strings.Contains(o.Data["name"], term) {
			out = append(out, o)
		}
	}

	return out, nil
}

func (db *pgDB) GetAccountAvatar(account Account) (*Object, error) {
	ctx := context.Background()
	obj := &Object{
		OwnerID: account.ID,
		Avatar:  true,
	}
	stmt := `
		SELECT id, data, script
		FROM objects
		WHERE owner = $1 AND avatar IS true`
	err := db.pool.QueryRow(ctx, stmt, account.ID).Scan(
		&obj.ID, &obj.Data, &obj.Script)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (db *pgDB) ActiveSessions() (out []Session, err error) {
	stmt := `SELECT id, account FROM sessions`
	rows, err := db.pool.Query(context.Background(), stmt)
	if err != nil {
		return
	}

	for rows.Next() {
		s := Session{}
		if err = rows.Scan(&s.ID, &s.AccountID); err != nil {
			return
		}
		out = append(out, s)
	}

	return
}

func (db *pgDB) ClearSessions() (err error) {
	_, err = db.pool.Exec(context.Background(), "DELETE FROM sessions")
	return
}

func hasInvocation(obj *Object) string {
	hi := "has({\n"
	for k, v := range obj.Data {
		hi += fmt.Sprintf(`%s = "%s",`, k, v) + "\n"
	}
	hi += "})"

	return hi
}

func (db *pgDB) CreateObject(owner *Account, obj *Object) error {
	ctx := context.Background()
	stmt := `
		INSERT INTO objects (avatar, bedroom, data, script, owner)
		VALUES ( $1, $2, $3, $4, $5)
		RETURNING id
	`

	obj.Script = hasInvocation(obj) + obj.Script

	err := db.pool.QueryRow(ctx, stmt,
		obj.Avatar, obj.Bedroom, obj.Data, obj.Script, owner.ID).Scan(
		&obj.ID)
	if err != nil {
		return err
	}

	return nil
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
