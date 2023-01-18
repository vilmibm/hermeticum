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

type ScriptAPI interface {
	Tell(int, int, string)
	Show(int, int, string)
	DB() db.DB
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
	scriptAPI ScriptAPI
}

func NewScriptContext(sAPI ScriptAPI) (*ScriptContext, error) {
	sc := &ScriptContext{
		scriptAPI: sAPI,
	}
	sc.incoming = make(chan VerbContext)

	go sc.Go()

	return sc, nil
}

func (sc *ScriptContext) Handle(vc VerbContext) {
	sc.incoming <- vc
}

func (sc *ScriptContext) Go() {
	var (
		l           *lua.LState
		err         error
		vc          VerbContext
		senderT     *lua.LTable
		handlers    *lua.LTable
		pattern     *regexp.Regexp
		handlerCall string
	)

	for {
		vc = <-sc.incoming
		if vc.Target.Script != sc.script {
			sc.script = vc.Target.Script
			l, err = newWitchEngine(sc.scriptAPI, vc.Target)
			if err != nil {
				log.Printf("error parsing script %s: %s", sc.script, err.Error())
				continue
			}

			// TODO clear this object out of the exits table
		}

		// TODO check execute permission and bail out potentially

		senderT = l.NewTable()
		senderT.RawSetString("name", lua.LString(vc.Sender.Data["name"]))
		senderT.RawSetString("ID", lua.LNumber(vc.Sender.ID))
		l.SetGlobal("sender", senderT)
		l.SetGlobal("msg", lua.LString(vc.Rest))

		handlers = l.GetGlobal("_handlers").(*lua.LTable)
		handlers.ForEach(func(k, v lua.LValue) {
			if k.String() != vc.Verb {
				return
			}
			v.(*lua.LTable).ForEach(func(kk, vv lua.LValue) {
				pattern = regexp.MustCompile(kk.String())
				if !pattern.MatchString(vc.Rest) {
					return
				}
				// TODO TODO TODO TODO TODO
				// this could be a remote code execution vuln; but by being here, I
				// believe vc.Verb has been effectively validated as "not a pile of
				// lua code" since it matched a handler.
				handlerCall = fmt.Sprintf(`_handlers.%s["%s"]()`, vc.Verb, pattern)
				if err = l.DoString(handlerCall); err != nil {
					log.Println(err)
				}
			})
		})
	}
}
