package client

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"git.sr.ht/~rockorager/vaxis"
	"git.sr.ht/~rockorager/vaxis/widgets/term"
	"git.sr.ht/~rockorager/vaxis/widgets/textinput"
	"github.com/vilmibm/hermeticum/proto"
	"google.golang.org/grpc"
)

var Quit = make(chan struct{})

type State struct {
	vx       *vaxis.Vaxis
	prompt   *textinput.Model
	room     *proto.Object
	contents []*proto.Object
	stream   grpc.BidiStreamingClient[proto.Command, proto.WorldEvent]
}

func New() (state State, err error) {
	vx, err := vaxis.New(vaxis.Options{
		DisableMouse: true,
	})
	if err != nil {
		return
	}
	prompt := textinput.New()
	prompt.SetPrompt("> ")
	state = State{
		vx:     vx,
		prompt: prompt,
	}
	state.vx.SetTitle("hermeticum")
	go func() {
		for event := range state.vx.Events() {
			state.vx.Window().Clear()
			state.handleEvent(event)
		}
	}()
	return
}

func (state *State) handleEvent(event vaxis.Event) {
	if key, ok := event.(vaxis.Key); ok {
		switch key.String() {
		case "Ctrl+c":
			close(Quit)
		case "Enter":
			// TODO: save command history for up press
			state.processInput()
		default:
			state.prompt.Update(event)
		}
	}

	win := state.vx.Window()
	w, h := win.Size()
	state.prompt.Draw(win.New(0, h-1, w, 1))
	state.vx.Render()
}

func (state *State) processInput() {
	defer state.prompt.SetContent("")
	msg := state.prompt.String()

	// TODO lol clean this up it's hideous
	var verb string
	rest := msg
	if strings.HasPrefix(msg, "/") {
		verb, rest, _ = strings.Cut(msg[1:], " ")
	} else {
		verb = "say"
	}

	if verb == "edit" {
		var o *proto.Object
		id, err := strconv.Atoi(rest)
		if err == nil {
			o = resolveObjectById(state.contents, id)
		}

		if o == nil {
			matches := resolveObjectByString(state.contents, rest)
			if len(matches) == 1 {
				o = matches[0]
			} else if len(matches) > 1 {
				// bleh tech demo
				panic("error: non unique item")
			}
		}

		if o == nil {
			// bleh tech demo
			panic("error: no such object in sight...")
		}

		// TODO lock object

		f, err := os.CreateTemp("", fmt.Sprintf("hermeticum-%s-*.lua", o.GetName()))
		if err != nil {
			// bleh tech demo
			panic("error: couldn't create temp file for editing")
		}
		f.WriteString(o.GetScript())

		state.vx.HideCursor()
		vt := term.New()
		vt.TERM = os.Getenv("TERM")
		vt.Attach(state.vx.PostEvent)
		vt.Focus()
		err = vt.Start(exec.Command("/usr/bin/vim", f.Name()))
		if err != nil {
			panic(err)
		}
		defer vt.Close()

		for ev := range state.vx.Events() {
			switch ev.(type) {
			case term.EventClosed:
				state.vx.HideCursor()
				state.vx.Window().Clear()
				// do all the editorFinishedMsg things heeeere
				return
			case vaxis.Redraw:
				vt.Draw(state.vx.Window())
				state.vx.Render()
				continue
			}

			vt.Update(ev)
		}
	}

	switch verb {
	case "quit", "q":
		close(Quit)
	default:
		cmd := &proto.Command{
			Verb: verb,
			Rest: rest,
		}
		state.stream.Send(cmd)
	}
}

func (state *State) Close() {
	state.vx.Close()
}
