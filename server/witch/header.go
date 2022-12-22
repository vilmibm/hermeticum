package witch

import (
	"github.com/vilmibm/hermeticum/server/db"
	lua "github.com/yuin/gopher-lua"
)

func hasWrapper(obj db.Object) func(*lua.LState) int {
	return func(ls *lua.LState) int {
		//lv := ls.ToTable(1)
		return 0
	}
}

func hearsWrapper(obj db.Object) func(*lua.LState) int {
	return func(ls *lua.LState) int {
		// TODO get handler from _handlers
		// TODO call it
		// TODO how to get message in here?

		return 0
	}
}

func does(ls *lua.LState) int {
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

func addHandler(ls *lua.LState) int {
	verb := ls.ToString(1)
	pattern := ls.ToString(2)
	cb := ls.ToFunction(3)
	handlers := ls.GetGlobal("_handlers").(*lua.LTable)
	newHandler := ls.NewTable()
	newHandler.RawSetString(pattern, cb)
	handlerMap := handlers.RawGetString(verb).(*lua.LTable)
	handlerMap.RawSetString(verb, newHandler)

	return 0
}
