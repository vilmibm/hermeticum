package witch

import (
	"log"

	"github.com/vilmibm/hermeticum/server/db"
	lua "github.com/yuin/gopher-lua"
)

func hasWrapper(obj db.Object) func(*lua.LState) int {
	return func(ls *lua.LState) int {
		lv := ls.ToTable(1)
		log.Printf("%#v", lv)
		return 0
	}
}

func hearsWrapper(obj db.Object) func(*lua.LState) int {
	return func(ls *lua.LState) int {
		// TODO get handler from _handlers
		// TODO call it
		// TODO how to get message in here?

		return 0
	}
}

func does(ls *lua.LState) int {
	// TODO
	return 0
}

const addHandler = `
_addHandler = function(verb, pattern, cb)
	_handlers[verb] = function(message)
		f, l = string.find(message, pattern)
		if f != nil
			cb(message)
		end
	end
end
`
