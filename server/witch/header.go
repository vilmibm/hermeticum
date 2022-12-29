package witch

import (
	"log"

	lua "github.com/yuin/gopher-lua"
)

func witchHas(l *lua.LState) int {
	lv := l.ToTable(1)
	log.Println(lv)
	return 0
}

func witchHears(l *lua.LState) int {
	// TODO register handler
	handlers := l.GetGlobal("_handlers").(*lua.LTable)
	log.Println(handlers)
	pattern := l.ToString(1)
	cb := l.ToFunction(2)
	addHandler(l, "say", pattern, cb)
	return 0
}

func witchDoes(ls *lua.LState) int {
	// TODO how to feed events back into the server?
	// it needs to behave like an event showing up in Commands stream
	// this handler needs a reference to the gateway which has a channel for sending events that the server will see?
	return 0
}

/*
	string -> fn does not work because there might be multiple handlers for a given verb.

	i can:
		- have a list of handlers. call each one. it is the handler's
		responsibility to decide if it's a match or not.
		- store string -> map[string]fn. do the matching in Go.

		handlers = {
			"hear" = {
				"*eat*" = cbfn0
				"*slurp*" = cbfn1
			}

			"see" = {
				"*fork*" = cbfn2
			}
		}
*/

func addHandler(l *lua.LState, verb, pattern string, cb *lua.LFunction) int {
	handlers := l.GetGlobal("_handlers").(*lua.LTable)

	verbHandlers, ok := handlers.RawGetString(verb).(*lua.LTable)
	if !ok {
		verbHandlers = l.NewTable()
		handlers.RawSetString(verb, verbHandlers)
	}

	log.Println("addHandler")
	log.Printf("%#v", cb)

	verbHandlers.RawSetString(pattern, cb)

	return 0
}
