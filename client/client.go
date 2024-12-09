package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
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
	messages viewport.Model
	state    viewport.Model
	stream   grpc.BidiStreamingClient[proto.Command, proto.WorldEvent]

	inbound chan *proto.WorldEvent
	ctx     context.Context
	err     error
}

func initialModel() model {
	prompt := textarea.New()
	prompt.Focus()
	prompt.Prompt = "> "
	prompt.SetWidth(100)
	prompt.SetHeight(1)
	prompt.FocusedStyle.CursorLine = lipgloss.NewStyle()

	prompt.ShowLineNumbers = false

	mvp := viewport.New(20, 30)

	prompt.KeyMap.InsertNewline.SetEnabled(false)
	inbound := make(chan *proto.WorldEvent)

	svp := viewport.New(20, 30)

	ctx := context.Background()

	return model{
		prompt:   prompt,
		messages: mvp,
		state:    svp,
		events:   []*proto.WorldEvent{},
		room:     nil,
		contents: []*proto.Object{},
		inbound:  inbound,
		ctx:      ctx,
	}
}

func (m model) listen() tea.Cmd {
	return func() tea.Msg {
		return <-m.inbound
	}
}

func (m model) connect() tea.Msg {
	gc, err := grpc.NewClient(
		"unix:///tmp/hermeticum.sock",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return tea.Quit
	}

	client := proto.NewGameWorldClient(gc)

	now := fmt.Sprintf("%d", time.Now().Unix())
	if _, err = client.Ping(
		m.ctx, &proto.PingMsg{When: now}); err != nil {
		return tea.Quit
	}

	stream, err := client.ClientInput(m.ctx)
	if err != nil {
		return fmt.Errorf("could not create command stream: %w", err)
	}

	go func() {
		for {
			if ev, err := stream.Recv(); err != nil {
				if err != io.EOF {
					m.err = err
				}
				break
			} else {
				m.inbound <- ev
			}
		}
	}()

	return stream
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.connect, m.listen())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case grpc.BidiStreamingClient[proto.Command, proto.WorldEvent]:
		m.stream = msg
		return m, nil
	case *proto.WorldEvent:
		if msg.Type != proto.WorldEvent_STATE {
			m.events = append(m.events, msg)
			vpContent := ""
			for _, e := range m.events {
				switch e.Type {
				case proto.WorldEvent_OVERHEARD:
					vpContent += fmt.Sprintf("%s: %s", e.GetSource(), e.GetText())
				case proto.WorldEvent_EMOTE:
					vpContent += fmt.Sprintf("%s %s", e.GetSource(), e.GetText())
				case proto.WorldEvent_PRINT:
					vpContent += fmt.Sprintf("%s", e.GetText())
				default:
					vpContent += fmt.Sprintf("%#v", e)
				}
				vpContent += "\n"
			}

			m.messages.SetContent(vpContent)
			m.messages.GotoBottom()
		} else {
			m.contents = msg.GetObjects()

			// TODO precompile this
			dt, err := template.New("details").Parse(detailsTmpl)
			if err != nil {
				panic(err)
			}

			update := bytes.NewBufferString("")
			err = dt.Execute(update, msg)
			if err != nil {
				m.err = err
			} else {
				m.state.SetContent(update.String())
			}
		}
		return m, m.listen()
	case tea.WindowSizeMsg:
		m.messages.Width = (msg.Width / 3) * 2
		m.state.Width = msg.Width / 3
		m.prompt.SetWidth(msg.Width)
		return m, nil
	case error:
		// TODO
		panic(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			v := m.prompt.Value()
			if v == "" {
				return m, nil
			}
			// TODO save command history for up press
			m.prompt.SetValue("")
			return m, m.processInput(v)
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

func (m model) processInput(value string) tea.Cmd {
	return func() tea.Msg {
		var verb string
		rest := value
		if strings.HasPrefix(value, "/") {
			verb, rest, _ = strings.Cut(value[1:], " ")
		} else {
			verb = "say"
		}
		cmd := &proto.Command{
			Verb: verb,
			Rest: rest,
		}
		return m.stream.Send(cmd)
	}
}

/* TODO

if len(os.Getenv("DEBUG")) > 0 {
	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
	defer f.Close()
}
*/

func (m model) View() string {
	stateStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("63"))
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, m.messages.View(), stateStyle.Render(m.state.View())),
		m.prompt.View())
}

type ConnectOpts struct {
}

type ClientState struct {
	Client       proto.GameWorldClient
	MaxMessages  int
	events       []*proto.WorldEvent
	currentRoom  *proto.Object
	roomContents []*proto.Object
	logger       *log.Logger
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

func Connect(opts ConnectOpts) error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
	//			if cmd.Verb == "edit" {
	//				var o *proto.Object
	//				id, err := strconv.Atoi(cmd.Rest)
	//				if err == nil {
	//					o = resolveObjectById(cs.roomContents, id)
	//				}

	//				if o == nil {
	//					matches := resolveObjectByString(cs.roomContents, cmd.Rest)
	//					if len(matches) == 1 {
	//						o = matches[0]
	//					} else if len(matches) > 1 {
	//						// TODO fuzzy error
	//						panic("non unique item")
	//					}
	//				}

	//				if o == nil {
	//					// TODO no such object error
	//					panic("no such dingus, dingus")
	//				}

	//				// TODO lock object

	//				editor := "/usr/bin/vim"
	//				if v := os.Getenv("VISUAL"); v != "" {
	//					editor = v
	//				} else if e := os.Getenv("EDITOR"); e != "" {
	//					editor = e
	//				}

	//				f, err := os.CreateTemp("", fmt.Sprintf("hermeticum-%s-*.lua", o.GetName()))
	//				if err != nil {
	//					// TODO
	//					panic(err)
	//				}
	//				// TODO as expected, this fails catastrophically.
	//				// TODO I'm considering switching to bubbletea, anyway. so i might give
	//				// up on external editors for now and just use their multi line editing
	//				// thing.
	//				cmd := exec.Command(editor, f.Name())
	//				cmd.Stdin = os.Stdin
	//				cmd.Stdout = os.Stdout
	//				err = cmd.Run()
	//				if err != nil {
	//					// TODO
	//					panic(err)
	//				}

	//				newContent, err := io.ReadAll(f)
	//				if err != nil {
	//					// TODO
	//					panic(err)
	//				}
	//				f.Close()

	//				cs.logger.Println(newContent)

	//				// TODO update object

	//				// TODO unlock object

	//			}
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
