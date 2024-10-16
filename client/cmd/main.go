package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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

var (
	serverAddr         = flag.String("addr", "localhost:6666", "The server address in the format of host:port")
	serverHostOverride = flag.String("server_host_override", "x.test.example.com", "The server name used to verify the hostname returned by the TLS handshake")
)

type ClientState struct {
	App          *tview.Application
	Client       proto.GameWorldClient
	SessionInfo  *proto.SessionInfo
	MaxMessages  int
	messagesView *tview.TextView
	messages     []*proto.ClientMessage
	cmdStream    proto.GameWorld_CommandsClient
}

func (cs *ClientState) Messages() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := cs.Client.Messages(ctx, cs.SessionInfo)
	if err != nil {
		return err
	}

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		cs.AddMessage(msg)
	}

	return nil
}

func (cs *ClientState) HandleSIGINT(sigC chan os.Signal) {
	for range sigC {
		cm := &proto.Command{
			SessionInfo: cs.SessionInfo,
			Verb:        "quit",
		}
		err := cs.cmdStream.Send(cm)
		if err != nil {
			fmt.Printf("failed to send quit verb to server: %s\n", err.Error())
		}
		_, err = cs.cmdStream.Recv()
		if err != nil {
			fmt.Printf("failed to receive an ACK from server: %s\n", err.Error())
		}

		cs.App.Stop()
	}
}

func (cs *ClientState) HandleInput(input string) error {
	var verb string
	rest := input
	if strings.HasPrefix(input, "/") {
		verb, rest, _ = strings.Cut(input[1:], " ")
	} else {
		verb = "say"
	}
	cmd := &proto.Command{
		SessionInfo: cs.SessionInfo,
		Verb:        verb,
		Rest:        rest,
	}
	// TODO I'm punting on handling CommandAcks for now but it will be a nice UX thing later for showing connectivity problems
	err := cs.cmdStream.Send(cmd)
	if err != nil {
		return err
	}
	_, err = cs.cmdStream.Recv()
	if err != nil {
		fmt.Printf("failed to receive an ACK from server: %s\n", err.Error())
	}
	if verb == "quit" || verb == "q" {
		cs.App.Stop()
	}
	return nil
}

func (cs *ClientState) InitCommandStream() error {
	ctx := context.Background()
	stream, err := cs.Client.Commands(ctx)
	if err != nil {
		return err
	}
	cs.cmdStream = stream
	return nil
}

func (cs *ClientState) AddMessage(msg *proto.ClientMessage) {
	// TODO i don't like this function
	cs.messages = append(cs.messages, msg)
	if len(cs.messages) > cs.MaxMessages {
		cs.messages = cs.messages[1 : len(cs.messages)-1]
	}

	// TODO look into using the SetChangedFunc thing.
	cs.App.QueueUpdateDraw(func() {
		// TODO trim content of messagesView /or/ see if tview has a buffer size that does it for me. use cs.messages to re-constitute.
		switch msg.Type {
		case proto.ClientMessage_OVERHEARD:
			fmt.Fprintf(cs.messagesView, "%s: %s\n", msg.GetSpeaker(), msg.GetText())
		case proto.ClientMessage_EMOTE:
			fmt.Fprintf(cs.messagesView, "%s %s\n", msg.GetSpeaker(), msg.GetText())
		default:
			fmt.Fprintf(cs.messagesView, "%#v\n", msg)
		}
		cs.messagesView.ScrollToEnd()
	})
}

func _main() error {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		return fmt.Errorf("fail to dial: %w", err)
	}
	defer conn.Close()

	client := proto.NewGameWorldClient(conn)

	app := tview.NewApplication()

	// TODO make a NewClientState
	// TODO rename this, like, UI
	cs := &ClientState{
		App:         app,
		SessionInfo: &proto.SessionInfo{},
		Client:      client,
		MaxMessages: 15, // TODO for testing
		messages:    []*proto.ClientMessage{},
	}

	err = cs.InitCommandStream()
	if err != nil {
		return fmt.Errorf("could not create command stream: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err = cs.Client.Ping(ctx, cs.SessionInfo); err != nil {
		log.Fatalf("%v.Ping -> %v", cs.Client, err)
	}

	pages := tview.NewPages()

	pages.AddPage("splash",
		tview.NewModal().
			AddButtons([]string{"hey. let's go"}).
			SetDoneFunc(func(_ int, _ string) {
				pages.SwitchToPage("main")
				app.ResizeToFullScreen(pages)
			}).SetText("h e r m e t i c u m"),
		true,
		true)

	commandInput := tview.NewInputField().SetLabel("> ")
	handleInput := func(_ tcell.Key) {
		input := commandInput.GetText()
		// TODO command history
		commandInput.SetText("")
		// TODO do i need to clear the input's text?
		go cs.HandleInput(input)
	}

	commandInput.SetDoneFunc(handleInput)

	mainPage := tview.NewList().
		AddItem("jack in", "connect using an existing account", '1', func() {
			pages.SwitchToPage("login")
		}).
		AddItem("rez a toon", "create a new account", '2', func() {
			pages.SwitchToPage("register")
		}).
		AddItem("open the hood", "client configuration", '3', nil).
		AddItem("get outta here", "quit the client", '4', func() {
			app.Stop()
		})

	pages.AddPage("main", mainPage, true, false)

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

	pages.AddPage("game", gamePage, true, false)

	return app.SetRoot(pages, true).SetFocus(pages).Run()
}

func main() {
	err := _main()
	if err != nil {
		log.Fatal(err.Error())
	}
}
