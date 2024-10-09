package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type Perm string

const (
	PermWorld  Perm = "world"
	PermOwner  Perm = "owner"
	baseScript      = ""
)

type Object struct {
	ID      int
	Avatar  bool
	Bedroom bool
	OwnerID int
	Perms   *Permissions
	script  string
	Data    map[string]string
}

type Permissions struct {
	Read  Perm
	Write Perm
	Carry Perm
	Exec  Perm
}

func NewObject(owneruid uint32) *Object {
	o := &Object{
		OwnerID: int(owneruid),
		Data:    map[string]string{},
	}

	o.SetData("name", "plain orb")
	o.SetData("description", "a smooth, dull, grey orb about the size of an eyeball.")

	o.SetScript(baseScript)

	o.Perms = &Permissions{
		Read:  PermWorld,
		Write: PermOwner,
		Carry: PermWorld,
		Exec:  PermWorld,
	}

	return o
}

func NewAvatar(owneruid uint32, username string) *Object {
	o := NewObject(owneruid)

	o.Perms.Carry = PermOwner

	o.SetData("name", username)
	o.SetData("description",
		fmt.Sprintf("a gaseous vapor. It smells faintly of %s.", randSmell()))

	o.SetScript(baseScript + `
		hears(".*", function()
			tellMe(msg)
		end)
		
		sees(".*", function()
			showMe(msg)
		end)`)

	o.Avatar = true

	return o
}

func NewBedroom(owneruid uint32, username string) *Object {
	o := NewObject(owneruid)

	o.Perms.Carry = PermOwner

	o.SetData("name", fmt.Sprintf("%s's bedroom", username))
	o.SetData("description", "an inner sanctum all your own.")

	o.Bedroom = true

	return o
}

func NewRoom(owneruid uint32) *Object {
	o := NewObject(owneruid)
	o.Perms.Carry = PermOwner
	o.SetData("name", "construction site")
	o.SetData("description", "a pist of moist dirt and rubble. surely it will become something.")

	return o
}

func (o *Object) SetData(key string, value string) {
	o.Data[key] = value
}

func (o *Object) GetData(key string) string {
	v, ok := o.Data[key]
	if !ok {
		return ""
	}
	return v
}

func (o *Object) SetScript(code string) {
	// TODO use a formatter
	o.script = strings.TrimSpace(code)
}

func (o *Object) AppendScript(code string) {
	o.script += "\n" + code
}

func (o *Object) GetScript() string {
	return fmt.Sprintf(`
		%s
		%s
		%s
	`, o.hasInvocation(), o.allowsInvocation(), o.script)
}

func (o *Object) hasInvocation() string {
	hi := "has({\n"
	for k, v := range o.Data {
		hi += fmt.Sprintf(`  %s = "%s",`, k, v) + "\n"
	}
	hi += "})"

	return hi
}

func (o *Object) allowsInvocation() string {
	return fmt.Sprintf(`
allows({
	read = "%s",
	write = "%s",
	carry = "%s",
	execute = "%s",
})`, o.Perms.Read, o.Perms.Write, o.Perms.Carry, o.Perms.Exec)
}

func (o *Object) Save(db *DB) error {
	ctx := context.Background()
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	stmt := `
		INSERT INTO objects (avatar, bedroom, data, script, owneruid)
		VALUES ( $1, $2, $3, $4, $5)
		RETURNING id`
	err = db.pool.QueryRow(ctx, stmt,
		o.Avatar, o.Bedroom, o.Data, o.script, o.OwnerID).Scan(
		&o.ID)
	if err != nil {
		return err
	}

	stmt = `INSERT INTO permissions (object, read, write, carry, exec)
					VALUES ($1, $2, $3, $4, $5)`
	if _, err = tx.Exec(ctx, stmt, o.ID,
		o.Perms.Read, o.Perms.Write, o.Perms.Carry, o.Perms.Exec); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (o *Object) Refresh(db *DB) error {
	s := `SELECT avatar, data, owneruid, script FROM objects WHERE id = $1`
	ctx := context.Background()

	err := db.pool.QueryRow(ctx, s, o.ID).Scan(
		&o.Avatar, &o.Data, &o.OwnerID, &o.script)

	if err != nil {
		return err
	}

	perms := &Permissions{}

	s = `SELECT read, write, carry, exec FROM permissions WHERE object = $1`
	err = db.pool.QueryRow(ctx, s, o.ID).Scan(
		&perms.Read, &perms.Write, &perms.Carry, &perms.Exec)
	if err != nil {
		return err
	}

	o.Perms = perms

	return nil
}
func (o *Object) Container(db *DB) (*Object, error) {
	s := "SELECT container FROM contains WHERE contained = $1"
	var containerID int
	err := db.pool.QueryRow(context.Background(), s, o.ID).Scan(&containerID)
	if err != nil {
		return nil, err
	}

	return db.ObjectByID(containerID)
}

func (o *Object) Earshot(db *DB) ([]*Object, error) {
	stmt := `
	SELECT id FROM objects WHERE
		id IN (SELECT contained FROM contains WHERE container = (
						SELECT container FROM contains WHERE contained = $1 LIMIT 1))
		OR id = (SELECT container FROM contains WHERE contained = $1 LIMIT 1)`
	rows, err := db.pool.Query(context.Background(), stmt, o.ID)
	if err != nil {
		return nil, err
	}

	out := []*Object{}

	for rows.Next() {
		var heardID int
		if err := rows.Scan(&heardID); err != nil {
			return nil, err
		}
		heard, err := db.ObjectByID(heardID)
		if err != nil {
			return nil, err
		}

		out = append(out, heard)
	}

	return out, nil
}

func Filter(os []*Object, term string) []*Object {
	out := []*Object{}

	id, err := strconv.Atoi(term)
	if err == nil {
		for _, o := range os {
			if o.ID == id {
				out = append(out, o)
			}
		}
	} else {
		for _, o := range os {
			if strings.Contains(o.Data["name"], term) {
				out = append(out, o)
			}
		}
	}

	return out
}

func (o *Object) MoveInto(db *DB, container Object) error {
	ctx := context.Background()
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	stmt := "DELETE FROM contains WHERE contained = $1"
	_, err = tx.Exec(ctx, stmt, o.ID)
	if err != nil {
		return err
	}

	stmt = "INSERT INTO contains (contained, container) VALUES ($1, $2)"
	_, err = tx.Exec(ctx, stmt, o.ID, container.ID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (o *Object) Contents(db *DB) ([]*Object, error) {
	stmt := `SELECT contained FROM contains WHERE container = $1`
	rows, err := db.pool.Query(context.Background(), stmt, o.ID)
	if err != nil {
		return nil, err
	}

	out := []*Object{}

	for rows.Next() {
		var containedID int
		if err := rows.Scan(&containedID); err != nil {
			return nil, err
		}
		contained, err := db.ObjectByID(containedID)
		if err != nil {
			return nil, err
		}

		out = append(out, contained)
	}

	return out, nil
}

func (o *Object) String() string {
	return fmt.Sprintf("%s (%d)", o.GetData("name"), o.ID)
}
