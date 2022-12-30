package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/vilmibm/hermeticum/proto"
	"github.com/vilmibm/hermeticum/server/db"
	"github.com/vilmibm/hermeticum/server/witch"
	"google.golang.org/grpc"
)

var (
	tls      = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	certFile = flag.String("cert_file", "", "The TLS cert file")
	keyFile  = flag.String("key_file", "", "The TLS key file")
	port     = flag.Int("port", 6666, "The server port")
)

func _main() (err error) {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		return err
	}

	var opts []grpc.ServerOption
	if *tls {
		log.Fatal("tls unsupported")
		/*
			// TODO base some stuff on the data package in the examples to get tls working
			if *certFile == "" {
				*certFile = data.Path("x509/server_cert.pem")
			}
			if *keyFile == "" {
				*keyFile = data.Path("x509/server_key.pem")
			}
			creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
			if err != nil {
				log.Fatalf("Failed to generate credentials %v", err)
			}
			opts = []grpc.ServerOption{grpc.Creds(creds)}
		*/
	}
	grpcServer := grpc.NewServer(opts...)
	srv, err := newServer()
	if err != nil {
		return err
	}
	proto.RegisterGameWorldServer(grpcServer, srv)
	grpcServer.Serve(l)

	return nil
}

type gameWorldServer struct {
	proto.UnimplementedGameWorldServer

	db             db.DB
	msgRouterMutex sync.Mutex
	msgRouter      map[string]func(*proto.ClientMessage) error
	scripts        map[int]*witch.ScriptContext
	scriptsMutex   sync.RWMutex
}

func newServer() (*gameWorldServer, error) {
	// TODO read from env or whatever
	db, err := db.NewDB("postgres://vilmibm:vilmibm@localhost:5432/hermeticum")
	if err != nil {
		return nil, err
	}

	if err = db.Ensure(); err != nil {
		return nil, fmt.Errorf("failed to ensure default entities: %w", err)
	}

	if err = db.ClearSessions(); err != nil {
		return nil, fmt.Errorf("could not clear sessions: %w", err)
	}

	s := &gameWorldServer{
		msgRouter:    make(map[string]func(*proto.ClientMessage) error),
		db:           db,
		scripts:      make(map[int]*witch.ScriptContext),
		scriptsMutex: sync.RWMutex{},
	}

	return s, nil
}

func (s *gameWorldServer) verbHandler(verb, rest string, sender, target db.Object) error {
	s.scriptsMutex.RLock()
	sc, ok := s.scripts[target.ID]
	s.scriptsMutex.RUnlock()
	var err error

	sid, _ := s.db.SessionIDForAvatar(target)
	serverAPI := witch.ServerAPI{
		Show: func(_ int, _ string) {},
		Tell: func(_ int, _ string) {},
	}
	if sid != "" {
		send := s.msgRouter[sid]
		getSenderName := func(senderID int) *string {
			senderName := "a mysterious stranger"

			sender, err := s.db.GetObjectByID(senderID)
			if err == nil {
				senderName = sender.Data["name"]
			} else {
				log.Println(err.Error())
			}

			return &senderName
		}
		serverAPI.Show = func(senderID int, msg string) {
			cm := proto.ClientMessage{
				Type:    proto.ClientMessage_EMOTE,
				Text:    msg,
				Speaker: getSenderName(senderID),
			}
			send(&cm)
		}
		serverAPI.Tell = func(senderID int, msg string) {
			cm := proto.ClientMessage{
				Type:    proto.ClientMessage_OVERHEARD,
				Text:    msg,
				Speaker: getSenderName(senderID),
			}
			send(&cm)
		}
	}

	if !ok || sc == nil {
		if sc, err = witch.NewScriptContext(serverAPI); err != nil {
			return err
		}

		s.scriptsMutex.Lock()
		s.scripts[target.ID] = sc
		s.scriptsMutex.Unlock()
	}

	vc := witch.VerbContext{
		Verb:   verb,
		Rest:   rest,
		Sender: sender,
		Target: target,
	}

	sc.Handle(vc)

	return nil
}

func (s *gameWorldServer) HandleCmd(verb, rest string, sender *db.Object) {
	// TODO
}

func (s *gameWorldServer) endSession(sid string) {
	var err error
	var avatar *db.Object
	log.Printf("ending session %s", sid)

	avatar, err = s.db.AvatarBySessionID(sid)
	if err != nil {
		log.Printf("error while ending session %s: %s", sid, err.Error())
	} else {
		s.scriptsMutex.Lock()
		delete(s.scripts, avatar.ID)
		s.scriptsMutex.Unlock()
	}

	if err = s.db.EndSession(sid); err != nil {
		log.Printf("error while ending session %s: %s", sid, err.Error())
	}

	delete(s.msgRouter, sid)

}

func (s *gameWorldServer) Commands(stream proto.GameWorld_CommandsServer) error {
	var sid string
	var cmd *proto.Command
	var err error
	var avatar *db.Object
	var send func(*proto.ClientMessage) error
	var affected []db.Object
	var o db.Object
	for {
		if cmd, err = stream.Recv(); err != nil {
			log.Printf("commands stream closed with error: %s", err.Error())
			return err
		}

		if sid == "" {
			sid = cmd.SessionInfo.SessionID
			defer s.endSession(sid)
		}

		if err = stream.Send(&proto.CommandAck{
			Acked: true,
		}); err != nil {
			log.Printf("unable to ack command in session %s", sid)
			return err
		}

		if send == nil {
			log.Printf("saving a send fn for session %s", sid)
			send = s.msgRouter[sid]
		}

		if avatar, err = s.db.AvatarBySessionID(sid); err != nil {
			return s.HandleError(send, err)
		}

		log.Printf("verb %s from avatar %d in session %s", cmd.Verb, avatar.ID, sid)

		if cmd.Verb == "quit" || cmd.Verb == "q" {
			return nil
		}

		if affected, err = s.db.Earshot(*avatar); err != nil {
			return s.HandleError(send, err)
		}

		for _, o = range affected {
			if err = s.verbHandler(cmd.Verb, cmd.Rest, *avatar, o); err != nil {
				log.Printf("error handling verb %s for object %d: %s", cmd.Verb, o.ID, err)
			}
		}

		//s.HandleCmd(cmd.Verb, cmd.Rest, avatar)
	}
}

func (s *gameWorldServer) Ping(ctx context.Context, _ *proto.SessionInfo) (*proto.Pong, error) {
	pong := &proto.Pong{
		When: "TODO",
	}

	return pong, nil
}

func (s *gameWorldServer) Messages(si *proto.SessionInfo, stream proto.GameWorld_MessagesServer) error {
	s.msgRouterMutex.Lock()
	s.msgRouter[si.SessionID] = stream.Send
	s.msgRouterMutex.Unlock()

	// TODO this is clearly bad but it works. I should refactor this so that messages are received on a channel.
	for {
	}
}

func (s *gameWorldServer) Register(ctx context.Context, auth *proto.AuthInfo) (si *proto.SessionInfo, err error) {
	var account *db.Account
	account, err = s.db.CreateAccount(auth.Username, auth.Password)
	if err != nil {
		return
	}

	var sessionID string
	sessionID, err = s.db.StartSession(*account)
	if err != nil {
		return nil, fmt.Errorf("failed to start session for %d: %w", account.ID, err)
	}
	log.Printf("started session for %s", account.Name)

	av, err := s.db.AvatarBySessionID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find avatar for %s: %w", sessionID, err)
	}

	foyer, err := s.db.GetObject("system", "foyer")
	if err != nil {
		return nil, fmt.Errorf("failed to find foyer: %w", err)
	}

	if err = s.db.MoveInto(*av, *foyer); err != nil {
		return nil, fmt.Errorf("failed to move %d into %d: %w", av.ID, foyer.ID, err)
	}

	// TODO send room info, avatar info to client (need to figure this out and update proto)

	si = &proto.SessionInfo{SessionID: sessionID}

	return
}

func (s *gameWorldServer) Login(ctx context.Context, auth *proto.AuthInfo) (si *proto.SessionInfo, err error) {
	var a *db.Account
	a, err = s.db.ValidateCredentials(auth.Username, auth.Password)
	if err != nil {
		return
	}

	var sessionID string
	sessionID, err = s.db.StartSession(*a)
	if err != nil {
		return
	}

	av, err := s.db.AvatarBySessionID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find avatar for %s: %w", sessionID, err)
	}

	foyer, err := s.db.GetObject("system", "foyer")
	if err != nil {
		return nil, fmt.Errorf("failed to find foyer: %w", err)
	}

	if err = s.db.MoveInto(*av, *foyer); err != nil {
		return nil, fmt.Errorf("failed to move %d into %d: %w", av.ID, foyer.ID, err)
	}

	si = &proto.SessionInfo{SessionID: sessionID}

	return
}

func (s *gameWorldServer) HandleError(send func(*proto.ClientMessage) error, err error) error {
	log.Printf("error: %s", err.Error())
	msg := &proto.ClientMessage{
		Type: proto.ClientMessage_WHISPER,
		Text: "server error :(",
	}
	err = send(msg)
	if err != nil {
		log.Printf("error sending to client: %s", err.Error())
	}
	return err
}

// TODO other server functions

func main() {
	err := _main()
	if err != nil {
		log.Fatal(err.Error())
	}
}
