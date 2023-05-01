package witch

/*

This file contains the definitions of functions that are injected into scope for WITCH scripts. See witch.go's ScriptContext to see how they are actually added to a LuaState.

TODO: consider making this (or witch.go) a different package entirely. the `witch` prefix for the function names in this file is a little annoying.

*/

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

func witchHas(l *lua.LState) int {
	l.SetGlobal("_has", l.ToTable(1))
	return 0
}

func witchProvides(l *lua.LState) int {
	// TODO test this manually

	verbAndPattern := l.ToString(1)
	l.Pop(1)

	split := strings.SplitN(verbAndPattern, " ", 2)
	verb := split[0]
	pattern := split[1]

	l.Push(lua.LString(pattern))

	return addPatternHandler(l, verb)
}

func witchHears(l *lua.LState) int {
	return addPatternHandler(l, "say")
}

func witchSees(l *lua.LState) int {
	return addPatternHandler(l, "emote")
}

func witchGoes(l *lua.LState) int {
	// arg 0: direction
	// arg 1: from room
	// arg 2: to room
	// TODO call addPatternHandler with "go" verb and direction pattern; figure out how to call moveSender etc
	// TODO call addPatternHandler again for the reverse direction
	return -1
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
