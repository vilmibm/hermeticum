package witch

import (
	lua "github.com/yuin/gopher-lua"
)

func witchHas(l *lua.LState) int {
	l.SetGlobal("_has", l.ToTable(1))
	return 0
}

// TODO provides

func witchHears(l *lua.LState) int {
	return addPatternHandler(l, "say")
}

func witchSees(l *lua.LState) int {
	return addPatternHandler(l, "emote")
}

func witchGo(l *lua.LState) int {
	// TODO get the handler map
	// - check if handler map has a Go handler already, exit early if so
	// TODO register this object as an exit in DB
	return addPatternHandler(l, "go")
}

func witchSeen(l *lua.LState) int {
	return addHandler(l, "look")
}

func witchMy(l *lua.LState) int {
	hasT := l.GetGlobal("_has").(*lua.LTable)
	val := hasT.RawGetString(l.ToString(1))
	l.Push(val)
	return 1
}

func witchDoes(ls *lua.LState) int {
	// TODO how to feed events back into the server?
	// it needs to behave like an event showing up in Commands stream
	// this handler needs a reference to the gateway which has a channel for sending events that the server will see?
	return 0
}

func addHandler(l *lua.LState, verb string) int {
	pattern := ".*"
	cb := l.ToFunction(1)

	//log.Printf("adding handler: %s %s %#v", verb, pattern, cb)

	handlers := l.GetGlobal("_handlers").(*lua.LTable)

	verbHandlers, ok := handlers.RawGetString(verb).(*lua.LTable)
	if !ok {
		verbHandlers = l.NewTable()
		handlers.RawSetString(verb, verbHandlers)
	}

	verbHandlers.RawSetString(pattern, cb)

	return 0
}

func addPatternHandler(l *lua.LState, verb string) int {
	pattern := l.ToString(1)
	cb := l.ToFunction(2)

	//log.Printf("adding handler: %s %s %#v", verb, string(pattern), cb)

	handlers := l.GetGlobal("_handlers").(*lua.LTable)

	verbHandlers, ok := handlers.RawGetString(verb).(*lua.LTable)
	if !ok {
		verbHandlers = l.NewTable()
		handlers.RawSetString(verb, verbHandlers)
	}

	verbHandlers.RawSetString(pattern, cb)

	return 0
}
