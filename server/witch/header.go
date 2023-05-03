package witch

/*

This file contains the definitions of functions that are injected into scope for WITCH scripts. See witch.go's ScriptContext to see how they are actually added to a LuaState.

TODO: consider making this (or witch.go) a different package entirely. the `witch` prefix for the function names in this file is a little annoying.

*/

import (
	"log"
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
	cb := l.ToFunction(2)

	split := strings.SplitN(verbAndPattern, " ", 2)
	verb := split[0]
	pattern := split[1]

	return addPatternHandler(l, verb, pattern, cb)
}

func witchHears(l *lua.LState) int {
	pattern := l.ToString(1)
	cb := l.ToFunction(2)
	return addPatternHandler(l, "say", pattern, cb)
}

func witchSees(l *lua.LState) int {
	pattern := l.ToString(1)
	cb := l.ToFunction(2)

	return addPatternHandler(l, "emote", pattern, cb)
}

func witchGoes(l *lua.LState) int {
	// TODO validate direction
	// TODO convert direction constant to english

	direction := l.ToString(1)
	targetRoom := l.ToString(2)

	cb := func(l *lua.LState) int {
		log.Printf("please move sender to target room '%s'", targetRoom)
		return 0
	}

	// TODO call addPatternHandler again for the reverse direction (make a reverse helper)
	return addPatternHandler(l, "go", direction, l.NewFunction(cb))
}

func witchSeen(l *lua.LState) int {
	cb := l.ToFunction(1)
	return addHandler(l, "look", cb)
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

func addHandler(l *lua.LState, verb string, cb *lua.LFunction) int {
	pattern := ".*"

	log.Printf("adding handler: %s %s %#v", verb, pattern, cb)

	handlers := l.GetGlobal("_handlers").(*lua.LTable)

	verbHandlers, ok := handlers.RawGetString(verb).(*lua.LTable)
	if !ok {
		verbHandlers = l.NewTable()
		handlers.RawSetString(verb, verbHandlers)
	}

	verbHandlers.RawSetString(pattern, cb)

	return 0
}

func addPatternHandler(l *lua.LState, verb, pattern string, cb *lua.LFunction) int {
	log.Printf("adding handler: %s %s %#v", verb, string(pattern), cb)

	handlers := l.GetGlobal("_handlers").(*lua.LTable)

	verbHandlers, ok := handlers.RawGetString(verb).(*lua.LTable)
	if !ok {
		verbHandlers = l.NewTable()
		handlers.RawSetString(verb, verbHandlers)
	}

	verbHandlers.RawSetString(pattern, cb)

	return 0
}
