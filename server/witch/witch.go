package witch

import (
	"log"

	"github.com/vilmibm/hermeticum/server/db"
	lua "github.com/yuin/gopher-lua"
)

/*

	the purpose of this package is to provide abstractions for sending verbs to game objects.

	Game objects get pulled from the DB into memory and their scripts become Lua States.

*/

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

type VerbContext struct {
	Verb   string
	Rest   string
	Sender db.Object
	Target db.Object
}

type ScriptContext struct {
	script   string
	incoming chan VerbContext
}

func NewScriptContext() (*ScriptContext, error) {
	sc := &ScriptContext{}
	sc.incoming = make(chan VerbContext)

	go func() {
		var l *lua.LState
		var err error
		var vc VerbContext
		for {
			vc = <-sc.incoming
			//if vc.Target.Script != sc.script {
			if dummyScript != sc.script {
				//sc.script = vc.Target.Script
				sc.script = dummyScript
				l = lua.NewState()
				l.SetGlobal("has", l.NewFunction(hasWrapper(vc.Target)))
				l.SetGlobal("hears", l.NewFunction(hearsWrapper(vc.Target)))
				l.SetGlobal("_handlers", l.NewTable())
				// TODO other setup
				//if err := l.DoString(obj.Script); err != nil {
				if err = l.DoString(dummyScript); err != nil {
					log.Printf("error parsing script %s: %s", dummyScript, err.Error())
				}
			}

			// TODO actually trigger the Lua script
		}
	}()

	return sc, nil
}

func (sc *ScriptContext) Handle(vc VerbContext) {
	log.Printf("%#v %#v", sc, vc)
	sc.incoming <- vc
}
