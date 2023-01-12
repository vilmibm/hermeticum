package witch

import (
	"fmt"
	"log"
	"regexp"

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

type ServerAPI struct {
	Tell func(int, int, string)
	Show func(int, int, string)
	DB   func() db.DB
}

type VerbContext struct {
	Verb   string
	Rest   string
	Sender db.Object
	Target db.Object
}

type ScriptContext struct {
	script    string
	incoming  chan VerbContext
	serverAPI ServerAPI
}

func NewScriptContext(sAPI ServerAPI) (*ScriptContext, error) {
	sc := &ScriptContext{
		serverAPI: sAPI,
	}
	sc.incoming = make(chan VerbContext)

	go func() {
		var l *lua.LState
		var err error
		var vc VerbContext
		for {
			vc = <-sc.incoming
			if vc.Target.Script != sc.script {
				// TODO clear this object out of the exits table
				sc.script = vc.Target.Script
				l = lua.NewState()
				l.SetGlobal("has", l.NewFunction(witchHas))
				l.SetGlobal("hears", l.NewFunction(witchHears))
				l.SetGlobal("sees", l.NewFunction(witchSees))
				l.SetGlobal("go", l.NewFunction(witchGo))
				l.SetGlobal("seen", l.NewFunction(witchSeen))
				l.SetGlobal("my", l.NewFunction(witchMy))
				l.SetGlobal("_handlers", l.NewTable())
				if err := l.DoString(vc.Target.Script); err != nil {
					log.Printf("error parsing script %s: %s", vc.Target.Script, err.Error())
				}
			}

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
				db := sc.serverAPI.DB()
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

			handlers := l.GetGlobal("_handlers").(*lua.LTable)
			handlers.ForEach(func(k, v lua.LValue) {
				if k.String() != vc.Verb {
					return
				}
				v.(*lua.LTable).ForEach(func(kk, vv lua.LValue) {
					pattern := regexp.MustCompile(kk.String())
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
