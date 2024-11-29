package witch

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/vilmibm/hermeticum/proto"
	"github.com/vilmibm/hermeticum/server/db"
	lua "github.com/yuin/gopher-lua"
)

/*
has({
	name = "a room",
	description = "it's empty",
})

allows({
  read = "world",
  write = "owner"
  carry = "owner",
  execute = "world",
})

hears(".*eat.*", function()
  does("quivers nervously")
end)
`
*/

/*
VerbContext represents an event that was issued from one object (perhaps a
player avatar) and heard/seen by another object.
*/
type VerbContext struct {
	Verb     string
	Rest     string
	Sender   db.Object
	Receiver db.Object
}

/*
ScriptContext provides state for executing the script of a WITCH object.  Every
WITCH object should have at most one of these in memory able to receive events
on a channel; it is up to the server to handle ScriptContext creation and
book-keeping such as lazy creation and mutex'ed storage of contexts.
*/
type ScriptContext struct {
	db       *db.DB
	script   string
	incoming chan VerbContext
	lState   *lua.LState
	obj      db.Object
	server   GameWorldServer
}

type GameWorldServer interface {
	PrintTo(db.Object, string)
	SendTo(db.Object, *proto.WorldEvent)
	SendStateUpdate(db.Object)
	Show(int, int, string)
	Tell(int, int, string)
	DB() *db.DB
}

func NewScriptContext(s GameWorldServer, obj db.Object) (*ScriptContext, error) {
	sc := &ScriptContext{
		db:     s.DB(),
		server: s,
		obj:    obj,
		lState: lua.NewState(),
	}
	sc.incoming = make(chan VerbContext)

	return sc, nil
}

func (sc *ScriptContext) initLua(obj db.Object) error {
	l := sc.lState

	// direction constants
	l.SetGlobal("east", lua.LString(dirEast))
	l.SetGlobal("west", lua.LString(dirWest))
	l.SetGlobal("north", lua.LString(dirNorth))
	l.SetGlobal("south", lua.LString(dirSouth))
	l.SetGlobal("above", lua.LString(dirAbove))
	l.SetGlobal("below", lua.LString(dirBelow))
	l.SetGlobal("up", lua.LString(dirAbove))
	l.SetGlobal("down", lua.LString(dirBelow))

	// witch object behavior functions
	l.SetGlobal("allows", l.NewFunction(sc.wAllows))
	l.SetGlobal("has", l.NewFunction(sc.wHas))
	l.SetGlobal("hears", l.NewFunction(sc.wHears))
	l.SetGlobal("sees", l.NewFunction(sc.wSees))
	l.SetGlobal("goes", l.NewFunction(sc.wGoes))
	l.SetGlobal("seen", l.NewFunction(sc.wSeen))
	l.SetGlobal("my", l.NewFunction(sc.wMy))
	l.SetGlobal("provides", l.NewFunction(sc.wProvides))

	// witch helpers
	l.SetGlobal("_handlers", l.NewTable())
	l.SetGlobal("_ID", lua.LNumber(obj.ID))

	return l.DoString(obj.GetScript())
}

func (sc *ScriptContext) Run() {
	go func() {
		for {
			vc := <-sc.incoming
			if vc.Receiver.ID != sc.obj.ID {
				panic("wtf?")
			}
			if vc.Receiver.GetScript() != sc.script {
				if err := sc.initLua(vc.Receiver); err != nil {
					log.Printf("error parsing script %s: %s",
						vc.Receiver.GetScript(), err.Error())
				} else {
					sc.script = vc.Receiver.GetScript()
				}
			}

			l := sc.lState
			if l == nil {
				continue
			}

			// witch action functions relative to calling context

			l.SetGlobal("tellMe", l.NewFunction(func(l *lua.LState) int {
				sender := l.GetGlobal("sender").(*lua.LTable)
				senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))

				log.Printf("tellMe: %d %s", senderID, l.ToString(1))
				sc.server.Tell(senderID, vc.Receiver.ID, l.ToString(1))
				return 0
			}))

			l.SetGlobal("tellSender", l.NewFunction(func(l *lua.LState) int {
				sender := l.GetGlobal("sender").(*lua.LTable)
				senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))

				log.Printf("tellMe: %d %s", senderID, l.ToString(1))
				sc.server.Tell(vc.Receiver.ID, senderID, l.ToString(1))
				return 0
			}))

			// TODO commenting this out until I decide on UX for object short coding
			/*
				l.SetGlobal("moveSender", l.NewFunction(func(l *lua.LState) (ret int) {
					ret = 0
					sender := l.GetGlobal("sender").(*lua.LTable)
					senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))
					owner := l.ToString(1)
					name := l.ToString(2)
					db := sc.db
					senderObj, err := db.GetObjectByID(senderID)
					if err != nil {
						log.Println(err.Error())
						return
					}
					container, err := db.GetObject(owner, name)
					if err != nil {
						log.Println(err.Error())
						return
					}
					if err = senderObj.MoveInto(db, *container); err != nil {
						log.Println(err.Error())
					}

					return
				}))
			*/

			l.SetGlobal("showMe", l.NewFunction(func(l *lua.LState) int {
				sender := l.GetGlobal("sender").(*lua.LTable)
				senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))

				log.Printf("showMe: %d %s", senderID, l.ToString(1))
				sc.server.Show(senderID, vc.Receiver.ID, l.ToString(1))
				return 0
			}))

			// TODO showSender?

			// TODO check execute permission and bail out potentially
			//log.Printf("%#v", vc)

			senderT := l.NewTable()
			senderT.RawSetString("name", lua.LString(vc.Sender.Data["name"]))
			senderT.RawSetString("ID", lua.LNumber(vc.Sender.ID))
			l.SetGlobal("sender", senderT)
			l.SetGlobal("msg", lua.LString(vc.Rest))
			l.SetGlobal("_SENDERID", lua.LNumber(vc.Sender.ID))

			handlers := l.GetGlobal("_handlers").(*lua.LTable)
			handlers.ForEach(func(k, v lua.LValue) {
				log.Println("checking handler verbs", k)
				if k.String() != vc.Verb {
					return
				}
				v.(*lua.LTable).ForEach(func(kk, vv lua.LValue) {
					pattern := regexp.MustCompile(kk.String())
					log.Println("checking handler", kk.String(), vv, pattern)
					if pattern.MatchString(vc.Rest) {
						// TODO TODO TODO TODO TODO
						// this could be a remote code execution vuln; but by being here, I
						// believe vc.Verb has been effectively validated as "not a pile of
						// lua code" since it matched a handler. How thoroughly do I
						// validate the handlers, though?
						if err := l.DoString(fmt.Sprintf(`_handlers.%s["%s"]()`, vc.Verb, pattern)); err != nil {
							log.Println(err.Error())
						}
					}
				})
			})
			sc.server.SendStateUpdate(vc.Sender)
		}
	}()
}

func (sc *ScriptContext) Handle(vc VerbContext) {
	sc.incoming <- vc
}

func (sc *ScriptContext) addHandler(l *lua.LState, verb, pattern string, cb *lua.LFunction) {
	log.Printf("adding handler: %s %s %#v", verb, string(pattern), cb)

	handlers := l.GetGlobal("_handlers").(*lua.LTable)

	verbHandlers, ok := handlers.RawGetString(verb).(*lua.LTable)
	if !ok {
		verbHandlers = l.NewTable()
		handlers.RawSetString(verb, verbHandlers)
	}

	verbHandlers.RawSetString(pattern, cb)
}

func (sc *ScriptContext) wMy(l *lua.LState) int {
	hasT := l.GetGlobal("_has").(*lua.LTable)
	val := hasT.RawGetString(l.ToString(1))
	l.Push(val)
	return 1
}

func (sc *ScriptContext) wAllows(l *lua.LState) int {
	l.SetGlobal("_allows", l.ToTable(1))
	// TODO
	return 0
}

func (sc *ScriptContext) wHas(l *lua.LState) int {
	l.SetGlobal("_has", l.ToTable(1))
	// TODO
	return 0
}

func (sc *ScriptContext) wHears(l *lua.LState) int {
	pattern := l.ToString(1)
	cb := l.ToFunction(2)
	sc.addHandler(l, "say", pattern, cb)
	return 0
}

func (sc *ScriptContext) wSees(l *lua.LState) int {
	pattern := l.ToString(1)
	cb := l.ToFunction(2)

	sc.addHandler(l, "emote", pattern, cb)
	return 0
}

func (sc *ScriptContext) wSeen(l *lua.LState) int {
	cb := l.ToFunction(1)
	sc.addHandler(l, "look", ".*", cb)
	return 0
}

func (sc *ScriptContext) wDoes(ls *lua.LState) int {
	// TODO how to feed events back into the server?
	// it needs to behave like an event showing up in Commands stream
	// this handler needs a reference to the gateway which has a channel for sending events that the server will see?
	return 0
}

func (sc *ScriptContext) wProvides(l *lua.LState) int {
	// TODO test this manually

	verbAndPattern := l.ToString(1)
	cb := l.ToFunction(2)

	split := strings.SplitN(verbAndPattern, " ", 2)
	verb := split[0]
	pattern := split[1]

	sc.addHandler(l, verb, pattern, cb)
	return 0
}

func (sc *ScriptContext) wGoes(l *lua.LState) int {
	direction := newDirection(l.ToString(1))
	targetRoomID := l.ToInt(2)

	log.Printf("GOT DIRECTION %v", direction)

	cb := func(l *lua.LState) (ret int) {
		targetRoom, err := sc.db.GetObjectByID(targetRoomID)
		if err != nil {
			log.Printf("failed to find room %s", err.Error())
			return
		}
		msg := l.GetGlobal("msg").String()
		if !ValidDirection(msg) {
			log.Printf("invalid direction in cb %s", msg)
			return
		}
		normalized := NormalizeDirection(msg)

		sender, err := sc.getSenderFromState(l)
		if err != nil {
			log.Printf("failed to find sender %s", err.Error())
			return
		}

		if normalized.Equals(direction) {
			log.Printf("MOVING SENDER TO '%s'", targetRoom.Data["name"])
			// TODO error checking
			sender.MoveInto(sc.db, *targetRoom)
			sc.server.Tell(targetRoom.ID, sender.ID, fmt.Sprintf("you are now in %s", targetRoom.Data["name"]))
			// TODO tell other avatars that person appeared
		}
		return
	}

	sc.addHandler(l, "go", ".*", l.NewFunction(cb))
	return 0
}

func (sc *ScriptContext) getSenderFromState(l *lua.LState) (*db.Object, error) {
	lsID := lua.LVAsNumber(l.GetGlobal("_SENDERID"))

	return sc.db.GetObjectByID(int(lsID))
}
