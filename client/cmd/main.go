package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/rivo/tview"
	"github.com/vilmibm/hermeticum/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	tls                = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	caFile             = flag.String("ca_file", "", "The file containing the CA root cert file")
	serverAddr         = flag.String("addr", "localhost:6666", "The server address in the format of host:port")
	serverHostOverride = flag.String("server_host_override", "x.test.example.com", "The server name used to verify the hostname returned by the TLS handshake")
)

func messages(cs *ClientState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

type ClientState struct {
	App          *tview.Application
	Client       proto.GameWorldClient
	SessionInfo  *proto.SessionInfo
	MaxMessages  int
	messagesView *tview.TextView
	messages     []*proto.ClientMessage
}

func (cs *ClientState) AddMessage(msg *proto.ClientMessage) {
	// TODO i don't like this function
	cs.messages = append(cs.messages, msg)
	if len(cs.messages) > cs.MaxMessages {
		cs.messages = cs.messages[1 : len(cs.messages)-1]
	}

	cs.App.QueueUpdateDraw(func() {
		cs.messagesView.SetText("")

		for _, msg := range cs.messages {
			fmt.Fprintf(cs.messagesView, "%#v\n", msg)
		}
	})
}

func _main() error {
	var opts []grpc.DialOption
	if *tls {
		return errors.New("TODO tls unsupported")
		/*
			if *caFile == "" {
				*caFile = data.Path("x509/ca_cert.pem")
			}
			creds, err := credentials.NewClientTLSFromFile(*caFile, *serverHostOverride)
			if err != nil {
				log.Fatalf("Failed to create TLS credentials %v", err)
			}
			opts = append(opts, grpc.WithTransportCredentials(creds))
		*/
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		return fmt.Errorf("fail to dial: %w", err)
	}
	defer conn.Close()

	client := proto.NewGameWorldClient(conn)

	// TODO registration and login stuff

	app := tview.NewApplication()

	// TODO make a NewClientState
	cs := &ClientState{
		App:         app,
		SessionInfo: &proto.SessionInfo{},
		Client:      client,
		MaxMessages: 15, // TODO for testing
		messages:    []*proto.ClientMessage{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pong, err := cs.Client.Ping(ctx, cs.SessionInfo)
	if err != nil {
		log.Fatalf("%v.Ping -> %v", cs.Client, err)
	}

	//stream, err := messageStream(client, sessionInfo)

	log.Printf("%#v", pong)

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

	mainPage := tview.NewList().
		AddItem("jack in", "connect using an existing account", '1', func() {
			pages.SwitchToPage("game")
		}).
		AddItem("rez a toon", "create a new account", '2', nil).
		AddItem("open the hood", "client configuration", '3', nil).
		AddItem("get outta here", "quit the client", '4', func() {
			app.Stop()
		})

	pages.AddPage("main", mainPage, true, false)

	msgView := tview.NewTextView()
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
			tview.NewTextView().SetText("TODO detail window"),
			1, 1, 1, 1, 10, 10, false).
		AddItem(
			tview.NewTextView().SetText("TODO input"),
			2, 0, 1, 2, 1, 30, false)

	pages.AddPage("game", gamePage, true, false)

	go messages(cs)

	return app.SetRoot(pages, true).SetFocus(pages).Run()
}

func main() {
	err := _main()
	if err != nil {
		log.Fatal(err.Error())
	}
}
