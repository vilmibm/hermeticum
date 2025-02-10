package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"

	"git.sr.ht/~rockorager/vaxis"
	"git.sr.ht/~rockorager/vaxis/widgets/term"
	"git.sr.ht/~rockorager/vaxis/widgets/textinput"
	"github.com/vilmibm/hermeticum/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var Quit = make(chan struct{})

type State struct {
	vx       *vaxis.Vaxis
	prompt   *textinput.Model
	room     *proto.Object
	contents []*proto.Object
	stream   grpc.BidiStreamingClient[proto.Command, proto.WorldEvent]
	ctx      context.Context
	inbound  chan *proto.WorldEvent
	state    string
	messages []string
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
		vx:      vx,
		prompt:  prompt,
		ctx:     context.Background(),
		inbound: make(chan *proto.WorldEvent),
	}
	state.connect()
	state.vx.SetTitle("hermeticum")
	go func() {
		for event := range state.vx.Events() {
			state.vx.Window().Clear()
			state.handleEvent(event)
		}
	}()
	go func() {
		for event := range state.inbound {
			if event.Type == proto.WorldEvent_STATE {
				state.contents = event.GetObjects()

				// TODO precompile this
				dt, err := template.New("details").Parse(detailsTmpl)
				if err != nil {
					panic(err)
				}

				update := bytes.NewBufferString("")
				err = dt.Execute(update, event)
				if err != nil {
					panic(err)
				} else {
					state.state = update.String()
				}
			}

			var toShow string
			switch event.Type {
			case proto.WorldEvent_OVERHEARD:
				toShow = fmt.Sprintf("%s: %s", event.GetSource(), event.GetText())
			case proto.WorldEvent_EMOTE:
				toShow = fmt.Sprintf("%s %s", event.GetSource(), event.GetText())
			case proto.WorldEvent_PRINT:
				toShow = fmt.Sprintf("%s", event.GetText())
				// default:
				// 	toShow = fmt.Sprintf("%#v", event)
			}
			if toShow != "" {
				state.messages = append(state.messages, toShow)
			}

			state.vx.PostEvent(vaxis.Redraw{})
		}
	}()
	return
}

func (state *State) connect() {
	gc, err := grpc.NewClient(
		"unix:///tmp/hermeticum.sock",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err.Error()) // TODO
	}

	client := proto.NewGameWorldClient(gc)

	now := fmt.Sprintf("%d", time.Now().Unix())
	if _, err = client.Ping(
		state.ctx, &proto.PingMsg{When: now}); err != nil {
		// TODO
		panic(err.Error())
	}

	stream, err := client.ClientInput(state.ctx)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			if ev, err := stream.Recv(); err != nil {
				if err != io.EOF {
					panic(err)
				}
				break
			} else {
				state.inbound <- ev
			}
		}
	}()

	state.stream = stream
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

	// box-drawing
	for y := 0; y < h; y++ {
		setCell(state.vx, w/3*2, y, '│', vaxis.Style{})
	}
	for x := 0; x < w; x++ {
		setCell(state.vx, x, h-2, '─', vaxis.Style{})
	}
	setCell(state.vx, w/3*2, h-2, '┴', vaxis.Style{})

	// input
	state.prompt.Draw(win.New(0, h-1, w, 1))

	// state
	win.New(w/3*2+1, 0, w/3, h-2).Wrap(vaxis.Segment{Text: state.state})

	// messages
	win.New(0, 0, w/3*2, h-2).Wrap(vaxis.Segment{Text: strings.Join(state.messages, "\n")})

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

func setCell(vx *vaxis.Vaxis, x int, y int, r rune, st vaxis.Style) {
	vx.Window().SetCell(x, y, vaxis.Cell{
		Character: vaxis.Character{
			Grapheme: string([]rune{r}),
		},
		Style: st,
	})
}
