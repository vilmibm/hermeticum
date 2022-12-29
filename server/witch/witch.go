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

type VerbContext struct {
	Verb   string
	Rest   string
	Sender db.Object
	Target db.Object
}

type ScriptContext struct {
	script   string
	incoming chan VerbContext
	tell     func(int, string)
}

func NewScriptContext(tell func(int, string)) (*ScriptContext, error) {
	sc := &ScriptContext{
		tell: tell,
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
				l.SetGlobal("has", l.NewFunction(witchHas))
				l.SetGlobal("hears", l.NewFunction(witchHears))
				l.SetGlobal("_handlers", l.NewTable())
				if err := l.DoString(vc.Target.Script); err != nil {
					log.Printf("error parsing script %s: %s", vc.Target.Script, err.Error())
				}
			}

			l.SetGlobal("tellMe", l.NewFunction(func(l *lua.LState) int {
				// TODO not getting senderID properly here
				sender := l.GetGlobal("sender").(*lua.LTable)
				senderID := int(lua.LVAsNumber(sender.RawGetString("ID")))
				msg := l.ToString(1)
				log.Printf("%#v %s\n", sender, msg)
				log.Println(senderID)
				sc.tell(senderID, msg)
				return 0
			}))

			// TODO check execute permission and bail out potentially

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
	log.Printf("%#v %#v", sc, vc)
	sc.incoming <- vc
}
