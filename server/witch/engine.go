package witch

import (
	"log"

	"github.com/vilmibm/hermeticum/server/db"
	lua "github.com/yuin/gopher-lua"
)

func setHandler(l *lua.LState, verb, pattern string, cb *lua.LFunction) int {
	handlers := l.GetGlobal("_handlers").(*lua.LTable)

	verbHandlers, ok := handlers.RawGetString(verb).(*lua.LTable)
	if !ok {
		verbHandlers = l.NewTable()
		handlers.RawSetString(verb, verbHandlers)
	}

	verbHandlers.RawSetString(pattern, cb)

	return 0
}

func addHandler(l *lua.LState, verb string) int {
	pattern := ".*"
	cb := l.ToFunction(1)

	return setHandler(l, verb, pattern, cb)
}

func addPatternHandler(l *lua.LState, verb string) int {
	pattern := l.ToString(1)
	cb := l.ToFunction(2)

	return setHandler(l, verb, pattern, cb)
}

func newWitchEngine(sapi ScriptAPI, obj db.Object) (*lua.LState, error) {
	l := lua.NewState()

	l.SetGlobal("_handlers", l.NewTable())

	l.SetGlobal("has", l.NewFunction(func(l *lua.LState) int {
		l.SetGlobal("_has", l.ToTable(1))
		return 0
	}))

	l.SetGlobal("hears", l.NewFunction(func(l *lua.LState) int {
		return addPatternHandler(l, "say")
	}))

	l.SetGlobal("sees", l.NewFunction(func(l *lua.LState) int {
		return addPatternHandler(l, "emote")
	}))

	l.SetGlobal("seen", l.NewFunction(func(l *lua.LState) int {
		return addHandler(l, "look")
	}))

	l.SetGlobal("my", l.NewFunction(func(l *lua.LState) int {
		hasT := l.GetGlobal("_has").(*lua.LTable)
		val := hasT.RawGetString(l.ToString(1))
		l.Push(val)
		return 1
	}))

	l.SetGlobal("tellMe", l.NewFunction(func(l *lua.LState) int {
		sender := l.GetGlobal("sender").(*lua.LTable)
		senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))

		log.Printf("tellMe: %d %s", senderID, l.ToString(1))
		sapi.Tell(senderID, obj.ID, l.ToString(1))
		return 0
	}))

	l.SetGlobal("showMe", l.NewFunction(func(l *lua.LState) int {
		sender := l.GetGlobal("sender").(*lua.LTable)
		senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))

		log.Printf("showMe: %d %s", senderID, l.ToString(1))
		sapi.Show(senderID, obj.ID, l.ToString(1))
		return 0
	}))

	l.SetGlobal("tellSender", l.NewFunction(func(l *lua.LState) int {
		sender := l.GetGlobal("sender").(*lua.LTable)
		senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))

		log.Printf("tellMe: %d %s", senderID, l.ToString(1))
		sapi.Tell(obj.ID, senderID, l.ToString(1))
		return 0
	}))

	l.SetGlobal("moveSender", l.NewFunction(func(l *lua.LState) (ret int) {
		ret = 0
		sender := l.GetGlobal("sender").(*lua.LTable)
		senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))
		owner := l.ToString(1)
		name := l.ToString(2)
		db := sapi.DB()
		senderObj, err := db.GetObjectByID(senderID)
		if err != nil {
			log.Println(err.Error())
			return
		}
		container, err := db.GetObject(owner, name)
		if err != nil {
			log.Println(err)
			return
		}
		if err = db.MoveInto(*senderObj, *container); err != nil {
			log.Println(err)
		}

		return
	}))

	l.SetGlobal("go", l.NewFunction(func(l *lua.LState) int {
		// TODO get whether it's currently an exit
		// TODO get the handler map
		// - check if handler map has a Go handler already, exit early if so
		// TODO register this object as an exit in DB
		return addPatternHandler(l, "go")
	}))

	// TODO does (will need to be able to send commands to server)
	// TODO provides
	// TODO allows
	// TODO showSender?

	return l, l.DoString(obj.Script)
}
