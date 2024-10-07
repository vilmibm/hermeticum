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
		//"GRANT ALL ON SCHEMA public TO postgres",
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

	foyer, err := ObjectByOwnerName(db, rootuid, "foyer")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		foyer = NewRoom(rootuid)
		foyer.SetData("name", "foyer")
		foyer.SetData("description", "a big room. the ceiling is painted with constellations.")
		if err = foyer.Save(db); err != nil {
			return err
		}
	}

	egg, err := ObjectByOwnerName(db, rootuid, "floor egg")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		egg = NewObject(rootuid)
		egg.SetData("name", "floor egg")
		egg.SetData("description", "it's an egg and it's on the floor")
		egg.Perms.Carry = PermOwner
		if err = egg.Save(db); err != nil {
			return err
		}
	}

	pub, err := ObjectByOwnerName(db, rootuid, "pub")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		pub = NewRoom(rootuid)
		pub.SetData("name", "pub")
		pub.SetData("description", "a warm, cozy pub constructed of hard wood and brass")
		if err = pub.Save(db); err != nil {
			return err
		}
	}

	oakDoor, err := ObjectByOwnerName(db, rootuid, "oak door")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		oakDoor = NewObject(rootuid)
		oakDoor.SetData("name", "oak door")
		oakDoor.SetData("description", "a heavy oak door with a brass handle. an ornate sign says PUB.")
		oakDoor.AppendScript(fmt.Sprintf("goes(north, %d)", pub.ID))
		oakDoor.Perms.Carry = PermOwner
		if err = oakDoor.Save(db); err != nil {
			return err
		}
	}

	revOakDoor, err := ObjectByOwnerName(db, rootuid, "oak door out")
	if err != nil {
		// TODO actually check error. for now assuming it means does not exist
		revOakDoor = NewObject(rootuid)
		revOakDoor.SetData("name", "oak door out")
		revOakDoor.SetData("description", "a heavy oak door with a brass handle. an ornate sign says EXIT.")
		revOakDoor.AppendScript(fmt.Sprintf("goes(south, %d)", foyer.ID))
		revOakDoor.Perms.Carry = PermOwner
		if err = revOakDoor.Save(db); err != nil {
			return err
		}
	}

	db.MoveInto(*egg, *foyer)
	db.MoveInto(*oakDoor, *foyer)
	db.MoveInto(*revOakDoor, *pub)

	return nil
}

func (db *DB) GreateAvatar(uid uint32, name string) (av *Object, err error) {
	av, err = db.GetAvatarForUid(uid)
	// TODO actually check error. for now assuming it means does not exist
	if err == nil {
		return
	}

	av = NewAvatar(uid, name)
	if err = av.Save(db); err != nil {
		return
	}

	br := NewBedroom(uid, name)
	if err = br.Save(db); err != nil {
		return
	}

	return
}

func (db *DB) ContainerFor(o Object) (oo *Object, err error) {
	oo = &Object{}
	stmt := `
		SELECT id, avatar, data, owneruid, script FROM objects
		WHERE id IN (SELECT container FROM contains WHERE contained = $1)
	`
	err = db.pool.QueryRow(context.Background(), stmt, o.ID).Scan(
		&oo.ID, &oo.Avatar, &oo.Data, &oo.OwnerID, &oo.script)
	return
}

func (db *DB) GetAvatarForUid(uid uint32) (av *Object, err error) {
	av = &Object{}
	stmt := `
	SELECT id, avatar, data, owneruid, script FROM objects 
	WHERE avatar = true AND owneruid = $1`
	err = db.pool.QueryRow(context.Background(), stmt, uid).Scan(
		&av.ID, &av.Avatar, &av.Data, &av.OwnerID, &av.script)
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

func (db *DB) GetObjectByID(ID int) (*Object, error) {
	ctx := context.Background()
	obj := &Object{}
	stmt := `
		SELECT id, avatar, data, owneruid, script
		FROM objects
		WHERE id = $1`
	err := db.pool.QueryRow(ctx, stmt, ID).Scan(
		&obj.ID, &obj.Avatar, &obj.Data, &obj.OwnerID, &obj.script)
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
		&obj.ID, &obj.Avatar, &obj.Data, &obj.OwnerID, &obj.script)

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
			&o.script); err != nil {
			return nil, err
		}
		out = append(out, o)
	}

	return out, nil
}

func (db *DB) Resolve(vantage Object, term string) ([]Object, error) {
	stuff, err := vantage.Earshot(db)
	if err != nil {
		return nil, err
	}

	out := []Object{}

	for _, o := range stuff {
		if strings.Contains(o.Data["name"], term) {
			out = append(out, *o)
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
