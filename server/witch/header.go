package witch

import (
	"log"

	lua "github.com/yuin/gopher-lua"
)

func witchHas(l *lua.LState) int {
	lv := l.ToTable(1)
	log.Println(lv)
	// TODO
	return 0
}

// TODO provides

func witchHears(l *lua.LState) int {
	return addHandler(l, "say")
}

func witchSees(l *lua.LState) int {
	log.Println("adding handler for emote")
	return addHandler(l, "emote")
}

func witchDoes(ls *lua.LState) int {
	// TODO how to feed events back into the server?
	// it needs to behave like an event showing up in Commands stream
	// this handler needs a reference to the gateway which has a channel for sending events that the server will see?
	return 0
}

func addHandler(l *lua.LState, verb string) int {
	pattern := l.ToString(1)
	cb := l.ToFunction(2)

	handlers := l.GetGlobal("_handlers").(*lua.LTable)

	verbHandlers, ok := handlers.RawGetString(verb).(*lua.LTable)
	if !ok {
		verbHandlers = l.NewTable()
		handlers.RawSetString(verb, verbHandlers)
	}

	verbHandlers.RawSetString(pattern, cb)

	return 0
}
