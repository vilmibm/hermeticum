package witch

import (
	"github.com/vilmibm/hermeticum/server/db"
	lua "github.com/yuin/gopher-lua"
)

/*

	the purpose of this package is to provide abstractions for sending verbs to game objects.

	Game objects get pulled from the DB into memory and their scripts become Lua States.

*/

type ScriptContext struct {
	script   string
	l        *lua.LState
	incoming chan string
	// TODO whatever is needed to support calling a Go API
}

func (sc *ScriptContext) NeedsRefresh(obj db.Object) bool {
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

hears(".*", function(msg)
	tellMe(sender().name + " says " + msg)
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

func NewScriptContext(obj db.Object) (*ScriptContext, error) {
	l := lua.NewState()

	l.SetGlobal("has", l.NewFunction(hasWrapper(obj)))
	l.SetGlobal("_handlers", l.NewTable())

	//if err := l.DoString(obj.Script); err != nil {
	if err := l.DoString(dummyScript); err != nil {
		return nil, err
	}

	return &ScriptContext{}, nil
}

func (sc *ScriptContext) Handle(verb, rest string, sender, target db.Object) error {
	// TODO call _handle function from the Lstate
	return nil
}
