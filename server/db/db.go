package db

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"math/rand"
	"os/user"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

type Object struct {
	ID      int
	Avatar  bool
	Bedroom bool
	Data    map[string]string
	OwnerID int
	Script  string
}

type DB struct {
	pool *pgxpool.Pool
}

func Connect() (*pgx.Conn, error) {
	conn, err := pgx.Connect(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to database: %w", err)
	}

	return conn, nil
}

func Pool() (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("Unable to connect to database: %w", err)
	}

	return pool, nil
}

type ResetOpts struct {
	DB *DB
}

func Reset(opts ResetOpts) error {
	if err := opts.DB.Erase(); err != nil {
		return fmt.Errorf("failed to reset database: %w", err)
	}

	if err := opts.DB.Ensure(); err != nil {
		return fmt.Errorf("failed to ensure default entities: %w", err)
	}

	return nil
}

func NewDB() (*DB, error) {
	pool, err := Pool()
	if err != nil {
		return nil, err
	}
	return &DB{
		pool: pool,
	}, nil
}

// Erase fully destroys the database's contents, dropping all tables.
func (db *DB) Erase() (err error) {
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
func (db *DB) Ensure() error {
	// TODO this is sloppy but shrug
	_, err := db.pool.Exec(context.Background(), schema)
	u, err := user.Lookup("root")
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return err
	}

	rootuid := uint32(uid)

	// TODO for some reason, when the seen() callback runs for foyer we're calling the stub Tell instead of the sid-closured Tell. figure out why.

	roomScript := `
		seen(function()
			tellSender(my("description"))
		end)
	`

	foyer, err := db.GetObject(rootuid, "foyer")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		data := map[string]string{}
		data["name"] = "foyer"
		data["description"] = "a big room. the ceiling is painted with constellations."
		foyer = &Object{
			Data:   data,
			Script: roomScript,
		}
		if err = db.CreateObject(rootuid, foyer); err != nil {
			return err
		}
	}

	egg, err := db.GetObject(rootuid, "floor egg")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		data := map[string]string{}
		data["name"] = "floor egg"
		data["description"] = "it's an egg and it's on the floor."
		egg = &Object{
			Data:   data,
			Script: "",
		}
		if err = db.CreateObject(rootuid, egg); err != nil {
			return err
		}
	}

	pub, err := db.GetObject(rootuid, "pub")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		data := map[string]string{}
		data["name"] = "pub"
		data["description"] = "a warm pub constructed of hard wood and brass"
		pub = &Object{
			Data:   data,
			Script: roomScript,
		}
		if err = db.CreateObject(rootuid, pub); err != nil {
			return err
		}
	}

	oakDoor, err := db.GetObject(rootuid, "oak door")
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
		if err = db.CreateObject(rootuid, oakDoor); err != nil {
			return err
		}
	}

	sysAva, err := db.GreateAvatar(rootuid, "root")
	if err != nil {
		return fmt.Errorf("could not find or create avatar for system account: %w", err)
	}

	db.MoveInto(*sysAva, *foyer)
	db.MoveInto(*egg, *foyer)
	db.MoveInto(*oakDoor, *foyer)

	return nil
}

func (db *DB) GreateAvatar(uid uint32, name string) (av *Object, err error) {
	ctx := context.Background()
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	av, err = db.GetAvatarForUid(uid)
	// TODO actually check error. for now assuming it means does not exist
	if err == nil {
		return
	}

	data := map[string]string{}
	data["name"] = name
	data["description"] = fmt.Sprintf("a gaseous form. it smells faintly of %s.", randSmell())
	av = &Object{
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

	stmt := "INSERT INTO objects ( avatar, data, owneruid, script ) VALUES ( $1, $2, $3, $4 ) RETURNING id"
	err = tx.QueryRow(ctx, stmt, av.Avatar, av.Data, uid, av.Script).Scan(&av.ID)
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

	stmt = "INSERT INTO objects ( bedroom, data, owneruid, script ) VALUES ( $1, $2, $3, $4 ) RETURNING id"
	err = tx.QueryRow(ctx, stmt, bedroom.Bedroom, bedroom.Data, uid, bedroom.Script).Scan(&bedroom.ID)
	if err != nil {
		return
	}

	stmt = "INSERT INTO permissions (object) VALUES ( $1 )"
	_, err = tx.Exec(ctx, stmt, bedroom.ID)
	if err != nil {
		return
	}

	err = tx.Commit(ctx)
	if err != nil {
		return
	}

	return
}

func (db *DB) GetAvatarForUid(uid uint32) (av *Object, err error) {
	av = &Object{}
	stmt := `
	SELECT id, avatar, data, owneruid, script FROM objects 
	WHERE avatar = true AND owneruid = $1`
	err = db.pool.QueryRow(context.Background(), stmt, uid).Scan(
		&av.ID, &av.Avatar, &av.Data, &av.OwnerID, &av.Script)
	return
}

func (db *DB) Derez(uid uint32) (err error) {
	var o *Object
	if o, err = db.GetAvatarForUid(uid); err == nil {
		if _, err = db.pool.Exec(context.Background(),
			"DELETE FROM contains WHERE contained = $1", o.ID); err != nil {
			log.Printf("failed to remove avatar from room: %s", err.Error())
		}
	} else {
		log.Printf("failed to find avatar for uid %d: %s", uid, err.Error())
	}

	return
}

func (db *DB) MoveInto(toMove Object, container Object) error {
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

func (db *DB) Earshot(obj Object) ([]Object, error) {
	stmt := `
	SELECT id, avatar, bedroom, data, owneruid, script FROM objects
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

func (db *DB) GetObjectByID(ID int) (*Object, error) {
	ctx := context.Background()
	obj := &Object{}
	stmt := `
		SELECT id, avatar, data, owneruid, script
		FROM objects
		WHERE id = $1`
	err := db.pool.QueryRow(ctx, stmt, ID).Scan(
		&obj.ID, &obj.Avatar, &obj.Data, &obj.OwnerID, &obj.Script)
	return obj, err
}

func (db *DB) GetObject(owneruid uint32, name string) (obj *Object, err error) {
	ctx := context.Background()
	obj = &Object{}
	stmt := `
		SELECT id, avatar, data, owneruid, script
		FROM objects
		WHERE owneruid = $1 AND data['name'] = $2`
	err = db.pool.QueryRow(ctx, stmt, owneruid, fmt.Sprintf(`"%s"`, name)).Scan(
		&obj.ID, &obj.Avatar, &obj.Data, &obj.OwnerID, &obj.Script)

	return
}

func (db *DB) SearchObjectsByName(term string) ([]Object, error) {
	ctx := context.Background()

	stmt := `
		SELECT id, avatar, data, owneruid, script
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

func (db *DB) Resolve(vantage Object, term string) ([]Object, error) {
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

func (db *DB) GhostBust() error {
	stmt := "DELETE FROM contains WHERE contained IN (SELECT id FROM objects WHERE objects.avatar)"
	if _, err := db.pool.Exec(context.Background(), stmt); err != nil {
		return fmt.Errorf("failed to bust ghosts: %w", err)
	}
	return nil
}

func hasInvocation(obj *Object) string {
	hi := "has({\n"
	for k, v := range obj.Data {
		hi += fmt.Sprintf(`%s = "%s",`, k, v) + "\n"
	}
	hi += "})"

	return hi
}

func (db *DB) CreateObject(owneruid uint32, obj *Object) error {
	ctx := context.Background()
	stmt := `
		INSERT INTO objects (avatar, bedroom, data, script, owneruid)
		VALUES ( $1, $2, $3, $4, $5)
		RETURNING id
	`

	obj.Script = hasInvocation(obj) + obj.Script

	err := db.pool.QueryRow(ctx, stmt,
		obj.Avatar, obj.Bedroom, obj.Data, obj.Script, owneruid).Scan(
		&obj.ID)
	if err != nil {
		return err
	}

	return nil
}

func randSmell() string {
	smells := []string{
		"lavender",
		"petrichor",
		"juniper",
		"pine sap",
		"wood smoke",
	}
	ix := rand.Intn(len(smells))
	return smells[ix]
}
