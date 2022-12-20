package witch

import (
	"sync"

	"github.com/vilmibm/hermeticum/server/db"
	lua "github.com/yuin/gopher-lua"
)

/*

	the purpose of this package is to provide abstractions for sending verbs to game objects.

	Game objects get pulled from the DB into memory and their scripts become Lua States.

*/

type scriptContext struct {
	script   string
	l        *lua.LState
	incoming chan string
	// TODO whatever is needed to support calling a Go API
}

func (sc *scriptContext) NeedsRefresh(obj db.Object) bool {
	return sc.script != obj.Script
}

// TODO using a dummy script for now

const dummyScript = `
has({
	name = "spaghetti",
	description = "a plate of pasta covered in pomodoro sauce"
})

hears(".*eat.*", function(msg)
	does("quivers nervously")
end)
`

/*
allows({
  read = "world",
  write = "owner"
  carry = "owner",
  execute = "world",
})

hears(".*eat.*", function(msg)
  does("quivers nervously")
end)
`
*/

// TODO figure out channel stuff
// TODO figure out how to inject WITCH header
// 	 - do i inject from Go or prepend some Lua code?
// TODO figure out how the Lua code can affect Go and thus the database

func newScriptContext(obj db.Object) (*scriptContext, error) {
	l := lua.NewState()

	l.SetGlobal("has", l.NewFunction(hasWrapper(obj)))
	l.SetGlobal("_handlers", l.NewTable())

	//if err := l.DoString(obj.Script); err != nil {
	if err := l.DoString(dummyScript); err != nil {
		return nil, err
	}

	return &scriptContext{}, nil
}

type Gateway struct {
	// maps game object IDs to script contexts
	m  map[int]*scriptContext
	mu sync.RWMutex
}

func NewGateway() *Gateway {
	return &Gateway{
		m:  map[int]*scriptContext{},
		mu: sync.RWMutex{},
	}
}

// RefreshObj ensures that the script context for the given game object is running the latest code for the object's script
func (g *Gateway) RefreshObject(obj db.Object) error {
	g.mu.RLock()
	var sc *scriptContext

	if sc, ok := g.m[obj.ID]; ok {
		if !sc.NeedsRefresh(obj) {
			g.mu.RUnlock()
			return nil
		}
	}
	g.mu.RUnlock()

	sc, err := newScriptContext(obj)
	if err != nil {
		return err
	}

	g.mu.Lock()
	g.m[obj.ID] = sc
	g.mu.Unlock()

	return nil
}

func (g *Gateway) VerbHandler(msg string, sender, target db.Object) error {
	var sc *scriptContext
	g.mu.RLock()
	sc, ok := g.m[target.ID]
	g.mu.RUnlock()

	if !ok || sc.NeedsRefresh(target) {
		sc, err := newScriptContext(target)
		if err != nil {
			return err
		}

		g.mu.Lock()
		g.m[target.ID] = sc
		g.mu.Unlock()
	}

	return nil
}
