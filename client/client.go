package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vilmibm/hermeticum/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type model struct {
	prompt   textarea.Model
	events   []*proto.WorldEvent
	room     *proto.Object
	contents []*proto.Object
	viewport viewport.Model
}

func initialModel() model {
	prompt := textarea.New()
	prompt.Placeholder = "lol"
	prompt.Focus()
	prompt.Prompt = "> "
	prompt.SetWidth(80)
	prompt.SetHeight(3)
	prompt.FocusedStyle.CursorLine = lipgloss.NewStyle()

	prompt.ShowLineNumbers = false

	vp := viewport.New(80, 20)
	vp.SetContent("^_^")
	prompt.KeyMap.InsertNewline.SetEnabled(false)

	return model{
		prompt:   prompt,
		viewport: vp,
		events:   []*proto.WorldEvent{},
		room:     nil,
		contents: []*proto.Object{},
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.prompt.SetWidth(msg.Width)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			v := m.prompt.Value()
			if v == "" {
				return m, nil
			}
			// TODO custom command to server send
			return m, nil
		default:
			var cmd tea.Cmd
			m.prompt, cmd = m.prompt.Update(msg)
			return m, cmd
		}
	case cursor.BlinkMsg:
		var cmd tea.Cmd
		m.prompt, cmd = m.prompt.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m model) View() string {
	return fmt.Sprintf(
		"%s\n\n%s",
		m.viewport.View(),
		m.prompt.View(),
	) + "\n\n"
}

type ConnectOpts struct {
}

type ClientState struct {
	//App          *tview.Application
	//details      *tview.TextView
	Client      proto.GameWorldClient
	MaxMessages int
	//messagesView *tview.TextView
	events       []*proto.WorldEvent
	cio          *clientIO
	currentRoom  *proto.Object
	roomContents []*proto.Object
	logger       *log.Logger
}

func (cs *ClientState) HandleInput(input string) {
	var verb string
	rest := input
	if strings.HasPrefix(input, "/") {
		verb, rest, _ = strings.Cut(input[1:], " ")
	} else {
		verb = "say"
	}
	cmd := &proto.Command{
		Verb: verb,
		Rest: rest,
	}
	cs.cio.outbound <- cmd
}

func (cs *ClientState) handleInbound(ev *proto.WorldEvent) {
	if ev.Type != proto.WorldEvent_STATE {
		cs.AddMessage(ev)
		return
	}
	cs.handleStateUpdate(ev)
}

const detailsTmpl = `{{.Room.Name}}
{{.Room.Description}}

{{range .Objects -}}
{{if .Avatar -}}
- *{{.Name}}
{{else -}}
- {{.Name}}
{{end -}}
{{end}}
`

func (cs *ClientState) handleStateUpdate(ev *proto.WorldEvent) {
	dt, err := template.New("details").Parse(detailsTmpl)
	if err != nil {
		panic(err)
	}

	update := bytes.NewBufferString("")
	err = dt.Execute(update, ev)
	if err != nil {
		cs.logger.Printf("failed to render details template: %s", err.Error())
	}
	cs.roomContents = ev.GetObjects()
	//cs.App.QueueUpdateDraw(func() {
	//	cs.details.SetText(update.String())
	//})
}

func (cs *ClientState) AddMessage(ev *proto.WorldEvent) {
	// TODO i don't like this function
	cs.events = append(cs.events, ev)
	if len(cs.events) > cs.MaxMessages {
		cs.events = cs.events[1 : len(cs.events)-1]
	}

	// TODO look into using the SetChangedFunc thing.
	//cs.App.QueueUpdateDraw(func() {
	//	// TODO trim content of messagesView /or/ see if tview has a buffer size that does it for me. use cs.messages to re-constitute.
	//	switch ev.Type {
	//	case proto.WorldEvent_OVERHEARD:
	//		fmt.Fprintf(cs.messagesView, "%s: %s\n", ev.GetSource(), ev.GetText())
	//	case proto.WorldEvent_EMOTE:
	//		fmt.Fprintf(cs.messagesView, "%s %s\n", ev.GetSource(), ev.GetText())
	//	case proto.WorldEvent_PRINT:
	//		fmt.Fprintf(cs.messagesView, "%s\n", ev.GetText())
	//	default:
	//		fmt.Fprintf(cs.messagesView, "%#v\n", ev)
	//	}
	//	cs.messagesView.ScrollToEnd()
	//})
	/*
		for _, ev := range cs.events {
			fmt.Print("\x1b[1B")
			switch ev.Type {
			case proto.WorldEvent_OVERHEARD:
				fmt.Printf("%s: %s\n", ev.GetSource(), ev.GetText())
			case proto.WorldEvent_EMOTE:
				fmt.Printf("%s %s\n", ev.GetSource(), ev.GetText())
			default:
				fmt.Printf("%#v\n", ev)
			}
		}
	*/
}

type clientIO struct {
	inbound  chan *proto.WorldEvent
	outbound chan *proto.Command
	errs     chan error
	done     chan bool
}

func Connect(opts ConnectOpts) error {
	gc, err := grpc.NewClient(
		"unix:///tmp/hermeticum.sock",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	client := proto.NewGameWorldClient(gc)
	//app := tview.NewApplication()

	cio := &clientIO{
		inbound:  make(chan *proto.WorldEvent),
		outbound: make(chan *proto.Command),
		errs:     make(chan error, 1),
		done:     make(chan bool, 1),
	}

	// TODO make a NewClientState
	// TODO rename this, like, UI
	cs := &ClientState{
		//App:         app,
		Client:      client,
		MaxMessages: 15, // TODO for testing
		events:      []*proto.WorldEvent{},
		cio:         cio,
		logger:      log.Default(),
	}

	now := fmt.Sprintf("%d", time.Now().Unix())

	if _, err = cs.Client.Ping(context.Background(), &proto.PingMsg{When: now}); err != nil {
		log.Fatalf("%v.Ping -> %v", cs.Client, err)
	}

	//commandInput := tview.NewInputField().SetLabel("> ")
	//handleInput := func(_ tcell.Key) {
	//	input := commandInput.GetText()
	//	// TODO command history
	//	commandInput.SetText("")
	//	// TODO do i need to clear the input's text?
	//	cs.HandleInput(input)
	//}

	//commandInput.SetDoneFunc(handleInput)

	// TODO need to hit ctrl c twice to quit but otherwise quitting works how i want
	//sigC := make(chan os.Signal, 1)
	//signal.Notify(sigC, os.Interrupt)

	//msgView := tview.NewTextView().SetScrollable(true).SetWrap(true).SetWordWrap(true)
	//cs.messagesView = msgView
	//cs.details = tview.NewTextView().SetText("...")

	//gamePage := tview.NewGrid().
	//	SetRows(1, 40, 3).
	//	SetColumns(-1, -1).
	//	SetBorders(true).
	//	AddItem(
	//		tview.NewTextView().SetTextAlign(tview.AlignLeft).SetText("h e r m e t i c u m"),
	//		0, 0, 1, 1, 1, 1, false).
	//	AddItem(
	//		tview.NewTextView().SetTextAlign(tview.AlignRight).SetText("TODO server status"),
	//		0, 1, 1, 1, 1, 1, false).
	//	AddItem(
	//		msgView,
	//		1, 0, 1, 1, 10, 20, false).
	//	AddItem(
	//		cs.details,
	//		1, 1, 1, 1, 10, 10, false).
	//	AddItem(
	//		commandInput,
	//		2, 0, 1, 2, 1, 30, false)

	//pages := tview.NewPages()
	//pages.AddPage("game", gamePage, true, true)

	ctx := context.Background()

	stream, err := cs.Client.ClientInput(ctx)
	if err != nil {
		return fmt.Errorf("could not create command stream: %w", err)
	}

	go func() {
		for {
			if ev, err := stream.Recv(); err != nil {
				cio.errs <- err
				cio.done <- true
			} else {
				cio.inbound <- ev
			}
		}
	}()

	//go func() {
	//	err := app.SetRoot(pages, true).SetFocus(commandInput).Run()
	//	if err != nil {
	//		cio.errs <- err
	//		cio.done <- true
	//	}
	//}()

	/*
		go func() {
			for {
				var s string
				r := bufio.NewReader(os.Stdin)
				for {
					fmt.Fprint(os.Stdout, "\x1b[H> ")
					s, _ = r.ReadString('\n')
					if s != "" {
						break
					}
				}
				cs.HandleInput(strings.TrimSpace(s))
			}
		}()
	*/

	//go func() {
	//	for range sigC {
	//		cmd := &proto.Command{
	//			Verb: "quit",
	//		}
	//		cs.cio.outbound <- cmd
	//	}
	//}()
	go func() {
		for {
			select {
			case ev := <-cio.inbound:
				cs.handleInbound(ev)
			case cmd := <-cio.outbound:
				if err := stream.Send(cmd); err != nil {
					cio.errs <- err
				}
				if cmd.Verb == "quit" {
					cio.done <- true
				}
				if cmd.Verb == "edit" {
					var o *proto.Object
					id, err := strconv.Atoi(cmd.Rest)
					if err == nil {
						o = resolveObjectById(cs.roomContents, id)
					}

					if o == nil {
						matches := resolveObjectByString(cs.roomContents, cmd.Rest)
						if len(matches) == 1 {
							o = matches[0]
						} else if len(matches) > 1 {
							// TODO fuzzy error
							panic("non unique item")
						}
					}

					if o == nil {
						// TODO no such object error
						panic("no such dingus, dingus")
					}

					// TODO lock object

					editor := "/usr/bin/vim"
					if v := os.Getenv("VISUAL"); v != "" {
						editor = v
					} else if e := os.Getenv("EDITOR"); e != "" {
						editor = e
					}

					f, err := os.CreateTemp("", fmt.Sprintf("hermeticum-%s-*.lua", o.GetName()))
					if err != nil {
						// TODO
						panic(err)
					}
					// TODO as expected, this fails catastrophically.
					// TODO I'm considering switching to bubbletea, anyway. so i might give
					// up on external editors for now and just use their multi line editing
					// thing.
					cmd := exec.Command(editor, f.Name())
					cmd.Stdin = os.Stdin
					cmd.Stdout = os.Stdout
					err = cmd.Run()
					if err != nil {
						// TODO
						panic(err)
					}

					newContent, err := io.ReadAll(f)
					if err != nil {
						// TODO
						panic(err)
					}
					f.Close()

					cs.logger.Println(newContent)

					// TODO update object

					// TODO unlock object

				}
			case err := <-cio.errs:
				log.Printf("error: %s", err.Error())
			case <-cio.done:
				// TODO this triggers data race warning when run with -race
				//cs.App.Stop() // TODO should this be in a defer
				//return nil
			}
		}
	}()

	p := tea.NewProgram(initialModel())
	_, err = p.Run()
	return err
}

func resolveObjectByString(objs []*proto.Object, s string) []*proto.Object {
	found := []*proto.Object{}
	for _, o := range objs {
		if strings.Contains(o.GetName(), s) {
			found = append(found, o)
		}
	}
	return found
}

func resolveObjectById(objs []*proto.Object, id int) *proto.Object {
	for _, o := range objs {
		if o.Id == uint64(id) {
			return o
		}
	}

	return nil
}
