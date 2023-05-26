package witch

/*

This file is the interface between the game server and WITCH execution

*/

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

type serverAPI struct {
	db      db.DB
	getSend func(string) func(*proto.ClientMessage) error
}

func (s *serverAPI) Tell(fromObjID, toObjID int, msg string) {
	log.Printf("Tell: %d %d %s", fromObjID, toObjID, msg)
	sid, err := s.db.SessionIDForObjID(toObjID)
	if err != nil {
		log.Println(err)
		return
	}

	if sid == "" {
		return
	}

	from, err := s.db.GetObjectByID(fromObjID)
	if err != nil {
		log.Println(err)
		return
	}

	speakerName := "an ethereal presence"
	if from.Data["name"] != "" {
		speakerName = from.Data["name"]
	}

	log.Println(sid)

	send := s.getSend(sid)
	cm := proto.ClientMessage{
		Type:    proto.ClientMessage_OVERHEARD,
		Text:    msg,
		Speaker: &speakerName,
	}
	send(&cm)
}

func (s *serverAPI) Show(fromObjID, toObjID int, action string) {
	sid, err := s.db.SessionIDForObjID(toObjID)
	if err != nil {
		log.Println(err.Error())
		return
	}

	from, err := s.db.GetObjectByID(fromObjID)
	if err != nil {
		log.Println(err)
		return
	}

	speakerName := "an ethereal presence"
	if from.Data["name"] != "" {
		speakerName = from.Data["name"]
	}

	log.Println(sid)

	send := s.getSend(sid)
	cm := proto.ClientMessage{
		Type:    proto.ClientMessage_EMOTE,
		Text:    action,
		Speaker: &speakerName,
	}
	send(&cm)
}

func (s *serverAPI) DB() db.DB {
	return s.db
}

type VerbContext struct {
	Verb   string
	Rest   string
	Sender db.Object
	Target db.Object
}

type ScriptContext struct {
	db        db.DB
	getSend   func(*proto.ClientMessage) error
	script    string
	incoming  chan VerbContext
	serverAPI serverAPI
}

func NewScriptContext(db db.DB, getSend func(string) func(*proto.ClientMessage) error) (*ScriptContext, error) {
	sc := &ScriptContext{
		serverAPI: serverAPI{db: db, getSend: getSend},
		db:        db,
	}
	sc.incoming = make(chan VerbContext)

	go func() {
		var l *lua.LState
		var err error
		var vc VerbContext
		for {
			vc = <-sc.incoming
			if vc.Target.Script != sc.script {
				sc.script = vc.Target.Script
				l = lua.NewState()

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
				l.SetGlobal("has", l.NewFunction(sc.wHas))
				l.SetGlobal("hears", l.NewFunction(sc.wHears))
				l.SetGlobal("sees", l.NewFunction(sc.wSees))
				l.SetGlobal("goes", l.NewFunction(sc.wGoes))
				l.SetGlobal("seen", l.NewFunction(sc.wSeen))
				l.SetGlobal("my", l.NewFunction(sc.wMy))
				l.SetGlobal("provides", l.NewFunction(sc.wProvides))

				// witch helpers
				l.SetGlobal("_handlers", l.NewTable())
				l.SetGlobal("_ID", lua.LNumber(vc.Target.ID))

				if err := l.DoString(vc.Target.Script); err != nil {
					log.Printf("error parsing script %s: %s", vc.Target.Script, err.Error())
				}
			}

			// witch action functions relative to calling context

			l.SetGlobal("tellMe", l.NewFunction(func(l *lua.LState) int {
				sender := l.GetGlobal("sender").(*lua.LTable)
				senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))

				log.Printf("tellMe: %d %s", senderID, l.ToString(1))
				sc.serverAPI.Tell(senderID, vc.Target.ID, l.ToString(1))
				return 0
			}))

			l.SetGlobal("tellSender", l.NewFunction(func(l *lua.LState) int {
				sender := l.GetGlobal("sender").(*lua.LTable)
				senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))

				log.Printf("tellMe: %d %s", senderID, l.ToString(1))
				sc.serverAPI.Tell(vc.Target.ID, senderID, l.ToString(1))
				return 0
			}))

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
				if err = db.MoveInto(*senderObj, *container); err != nil {
					log.Println(err.Error())
				}

				return
			}))

			l.SetGlobal("showMe", l.NewFunction(func(l *lua.LState) int {
				sender := l.GetGlobal("sender").(*lua.LTable)
				senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))

				log.Printf("showMe: %d %s", senderID, l.ToString(1))
				sc.serverAPI.Show(senderID, vc.Target.ID, l.ToString(1))
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
						// lua code" since it matched a handler.
						if err = l.DoString(fmt.Sprintf(`_handlers.%s["%s"]()`, vc.Verb, pattern)); err != nil {
							log.Println(err.Error())
						}
					}
				})
			})
		}
	}()

	return sc, nil
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

func (sc *ScriptContext) wHas(l *lua.LState) int {
	l.SetGlobal("_has", l.ToTable(1))
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
	direction := NewDirection(l.ToString(1))
	targetRoomTerm := l.ToString(2)

	log.Printf("GOT DIRECTION %v", direction)

	cb := func(l *lua.LState) (ret int) {
		targetRoomList, err := sc.db.SearchObjectsByName(targetRoomTerm)
		if err != nil {
			log.Printf("failed to search for target room: %s", err.Error())
			return
		}
		switch len(targetRoomList) {
		case 0:
			log.Printf("failed to find any matching target room. tell player somehow")
			return
		case 1:
			log.Printf("found the target room")
		default:
			log.Printf("found too many matching target rooms. tell player somehow")
			return
		}

		targetRoom := targetRoomList[0]
		msg := l.GetGlobal("msg").String()
		normalized, err := NormalizeDirection(msg)
		if err != nil {
			return
		}

		sender, err := sc.getSenderFromState(l)
		if err != nil {
			log.Printf("failed to find sender %s", err.Error())
			return
		}

		if normalized.Equals(direction) {
			log.Printf("MOVING SENDER TO '%s'", targetRoom.Data["name"])
			// TODO error checking
			sc.db.MoveInto(*sender, targetRoom)
			sc.serverAPI.Tell(targetRoom.ID, sender.ID, fmt.Sprintf("you are now in %s", targetRoom.Data["name"]))
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
