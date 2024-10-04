package client

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vilmibm/hermeticum/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ConnectOpts struct {
}

type ClientState struct {
	App          *tview.Application
	Client       proto.GameWorldClient
	MaxMessages  int
	messagesView *tview.TextView
	events       []*proto.WorldEvent
	cio          *clientIO
}

func (cs *ClientState) HandleSIGINT(sigC chan os.Signal) {
	for range sigC {
		cmd := &proto.Command{
			Verb: "quit",
		}
		cs.cio.outbound <- cmd
	}
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

func (cs *ClientState) AddMessage(ev *proto.WorldEvent) {
	// TODO i don't like this function
	cs.events = append(cs.events, ev)
	if len(cs.events) > cs.MaxMessages {
		cs.events = cs.events[1 : len(cs.events)-1]
	}

	// TODO look into using the SetChangedFunc thing.
	cs.App.QueueUpdateDraw(func() {
		// TODO trim content of messagesView /or/ see if tview has a buffer size that does it for me. use cs.messages to re-constitute.
		switch ev.Type {
		case proto.WorldEvent_OVERHEARD:
			fmt.Fprintf(cs.messagesView, "%s: %s\n", ev.GetSource(), ev.GetText())
		case proto.WorldEvent_EMOTE:
			fmt.Fprintf(cs.messagesView, "%s %s\n", ev.GetSource(), ev.GetText())
		default:
			fmt.Fprintf(cs.messagesView, "%#v\n", ev)
		}
		cs.messagesView.ScrollToEnd()
	})
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
	app := tview.NewApplication()

	// TODO make a NewClientState
	// TODO rename this, like, UI
	cs := &ClientState{
		App:         app,
		Client:      client,
		MaxMessages: 15, // TODO for testing
		events:      []*proto.WorldEvent{},
	}

	cio := &clientIO{
		inbound:  make(chan *proto.WorldEvent),
		outbound: make(chan *proto.Command),
		errs:     make(chan error),
		done:     make(chan bool),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := fmt.Sprintf("%d", time.Now().Unix())

	if _, err = cs.Client.Ping(context.Background(), &proto.PingMsg{When: now}); err != nil {
		log.Fatalf("%v.Ping -> %v", cs.Client, err)
	}

	commandInput := tview.NewInputField().SetLabel("> ")
	handleInput := func(_ tcell.Key) {
		input := commandInput.GetText()
		// TODO command history
		commandInput.SetText("")
		// TODO do i need to clear the input's text?
		cs.HandleInput(input)
	}

	commandInput.SetDoneFunc(handleInput)

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt)

	msgView := tview.NewTextView().SetScrollable(true).SetWrap(true).SetWordWrap(true)
	cs.messagesView = msgView
	gamePage := tview.NewGrid().
		SetRows(1, 40, 3).
		SetColumns(-1, -1).
		SetBorders(true).
		AddItem(
			tview.NewTextView().SetTextAlign(tview.AlignLeft).SetText("h e r m e t i c u m"),
			0, 0, 1, 1, 1, 1, false).
		AddItem(
			tview.NewTextView().SetTextAlign(tview.AlignRight).SetText("TODO server status"),
			0, 1, 1, 1, 1, 1, false).
		AddItem(
			msgView,
			1, 0, 1, 1, 10, 20, false).
		AddItem(
			tview.NewTextView().SetText("TODO details"),
			1, 1, 1, 1, 10, 10, false).
		AddItem(
			commandInput,
			2, 0, 1, 2, 1, 30, false)

	pages := tview.NewPages()
	pages.AddPage("game", gamePage, true, true)

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

	go func() {
		err := app.SetRoot(pages, true).SetFocus(commandInput).Run()
		if err != nil {
			cio.errs <- err
			cio.done <- true
		}
	}()

	for {
		select {
		case ev := <-cio.inbound:
			cs.AddMessage(ev)
		case cmd := <-cio.outbound:
			if err := stream.Send(cmd); err != nil {
				cio.errs <- err
			}
			if cmd.Verb == "quit" {
				cio.done <- true
			}
		case err := <-cio.errs:
			log.Printf("error: %s", err.Error())
		case <-cio.done:
			cs.App.Stop()
			return nil
		}
	}
}
