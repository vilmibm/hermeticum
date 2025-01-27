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

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	lg "github.com/charmbracelet/lipgloss"
	"github.com/vilmibm/hermeticum/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ConnectOpts struct {
}

type model struct {
	editor    string
	prompt    textarea.Model
	events    []*proto.WorldEvent // TODO needed?
	outputLog []string
	room      *proto.Object
	contents  []*proto.Object
	messages  viewport.Model
	state     viewport.Model
	stream    grpc.BidiStreamingClient[proto.Command, proto.WorldEvent]

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
	prompt.FocusedStyle.CursorLine = lg.NewStyle()

	prompt.ShowLineNumbers = false

	mvp := viewport.New(20, 30)

	prompt.KeyMap.InsertNewline.SetEnabled(false)
	inbound := make(chan *proto.WorldEvent)

	svp := viewport.New(20, 30)

	ctx := context.Background()

	editor := "/usr/bin/vim"
	if v := os.Getenv("VISUAL"); v != "" {
		editor = v
	} else if e := os.Getenv("EDITOR"); e != "" {
		editor = e
	}

	return model{
		editor:    editor,
		prompt:    prompt,
		messages:  mvp,
		state:     svp,
		events:    []*proto.WorldEvent{}, // TODO do i need this
		outputLog: []string{},
		room:      nil,
		contents:  []*proto.Object{},
		inbound:   inbound,
		ctx:       ctx,
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
		panic(err.Error()) // TODO
	}

	client := proto.NewGameWorldClient(gc)

	now := fmt.Sprintf("%d", time.Now().Unix())
	if _, err = client.Ping(
		m.ctx, &proto.PingMsg{When: now}); err != nil {
		// TODO
		panic(err.Error())
	}

	stream, err := client.ClientInput(m.ctx)
	if err != nil {
		return fmt.Errorf("could not create command stream: %w", err)
	}

	go func() {
		for {
			if ev, err := stream.Recv(); err != nil {
				if err != io.EOF {
					// TODO add server error to inbound?
					m.err = err // TODO what was i going to do with m.err
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

type show string

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		return m, func() tea.Msg { return show(fmt.Sprintf("shit: %s", m.err.Error())) }
	}
	switch msg := msg.(type) {
	case grpc.BidiStreamingClient[proto.Command, proto.WorldEvent]:
		m.stream = msg
		return m, nil
	case show:
		m.outputLog = append(m.outputLog, string(msg))
		vpContent := strings.Join(m.outputLog, "\n")
		vpContent += "\n"
		m.messages.SetContent(vpContent)
		m.messages.GotoBottom()
		return m, nil
	case *proto.WorldEvent:
		if msg.Type == proto.WorldEvent_STATE {
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
			return m, m.listen()
		}

		m.events = append(m.events, msg)
		var toShow show
		switch msg.Type {
		case proto.WorldEvent_OVERHEARD:
			toShow = show(fmt.Sprintf("%s: %s", msg.GetSource(), msg.GetText()))
		case proto.WorldEvent_EMOTE:
			toShow = show(fmt.Sprintf("%s %s", msg.GetSource(), msg.GetText()))
		case proto.WorldEvent_PRINT:
			toShow = show(fmt.Sprintf("%s", msg.GetText()))
		default:
			toShow = show(fmt.Sprintf("%#v", msg))
		}

		return m, tea.Batch(func() tea.Msg { return toShow }, m.listen())
	case tea.WindowSizeMsg:
		m.messages.Width = (msg.Width / 3) * 2
		m.state.Width = msg.Width / 3
		m.prompt.SetWidth(msg.Width - 2)
		return m, nil
	case error:
		// TODO
		panic("what" + msg.Error())
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
	case editorFinishedMsg:
		if msg.err != nil {
			return m, errMsg(msg.err)
		}
		msg.f.Seek(0, 0)
		newScript, err := io.ReadAll(msg.f)
		if err != nil {
			return m, errMsg(err)
		}
		msg.f.Close()
		sCmd := &proto.Command{
			Verb: "update",
			Rest: fmt.Sprintf("%d %s",
				msg.obj.Id,
				string(newScript),
			),
		}
		// TODO cmd to unlock object
		return m, func() tea.Msg { return m.stream.Send(sCmd) }
	default:
		return m, nil
	}
}

func errMsg(err error) func() tea.Msg {
	return func() tea.Msg {
		return show("error: " + err.Error())
	}
}

type editorFinishedMsg struct {
	err error
	obj *proto.Object
	f   *os.File
}

func (m model) processInput(value string) tea.Cmd {
	// TODO lol clean this up it's hideous
	var verb string
	rest := value
	if strings.HasPrefix(value, "/") {
		verb, rest, _ = strings.Cut(value[1:], " ")
	} else {
		verb = "say"
	}

	if verb == "edit" {
		var o *proto.Object
		id, err := strconv.Atoi(rest)
		if err == nil {
			o = resolveObjectById(m.contents, id)
		}

		if o == nil {
			matches := resolveObjectByString(m.contents, rest)
			if len(matches) == 1 {
				o = matches[0]
			} else if len(matches) > 1 {
				// TODO fancier error
				return func() tea.Msg { return show("error: non unique item") }
			}
		}

		if o == nil {
			return func() tea.Msg { return show("error: no such object in sight...") }
		}

		// TODO lock object

		f, err := os.CreateTemp("", fmt.Sprintf("hermeticum-%s-*.lua", o.GetName()))
		if err != nil {
			return func() tea.Msg { return show("error: couldn't create temp file for editing") }
		}
		f.WriteString(o.GetScript())
		cmd := exec.Command(m.editor, f.Name())
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			return editorFinishedMsg{err, o, f}
		})
	}

	return func() tea.Msg {
		switch verb {
		case "quit", "q":
			return tea.Quit()
		default:
			cmd := &proto.Command{
				Verb: verb,
				Rest: rest,
			}
			return m.stream.Send(cmd)
		}
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
	stateStyle := lg.NewStyle().
		BorderStyle(lg.NormalBorder()).
		BorderForeground(lg.Color("63"))
	promptStyle := lg.NewStyle().
		BorderStyle(lg.NormalBorder()).
		BorderForeground(lg.Color("63"))
	return lg.JoinVertical(lg.Left,
		lg.JoinHorizontal(lg.Top, m.messages.View(), stateStyle.Render(m.state.View())),
		promptStyle.Render(m.prompt.View()))
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

func Connect(opts ConnectOpts) error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err := p.Run()
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
