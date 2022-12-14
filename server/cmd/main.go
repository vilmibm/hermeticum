package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/vilmibm/hermeticum/proto"
	"github.com/vilmibm/hermeticum/server/db"
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
	fmt.Printf("DBG %#v\n", l)

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

	db        db.DB
	mu        sync.Mutex // for msgRouter
	msgRouter map[string]func(*proto.ClientMessage) error
}

func newServer() (*gameWorldServer, error) {
	// TODO read from env or whatever
	db, err := db.NewDB("postgres://vilmibm:vilmibm@localhost:5432/hermeticum")
	if err != nil {
		return nil, err
	}

	if err = db.ClearSessions(); err != nil {
		return nil, fmt.Errorf("could not clear sessions: %w", err)
	}

	s := &gameWorldServer{
		msgRouter: make(map[string]func(*proto.ClientMessage) error),
		db:        db,
	}

	return s, nil
}

func (s *gameWorldServer) Commands(stream proto.GameWorld_CommandsServer) error {
	var sid string
	for {
		cmd, err := stream.Recv()
		if err == io.EOF {
			// TODO this doesn't really do anything. if a client
			// disconnects without warning there's no EOF.
			return s.db.EndSession(sid)
		}
		if err != nil {
			return err
		}

		sid = cmd.SessionInfo.SessionID

		log.Printf("verb %s in session %s", cmd.Verb, sid)

		if cmd.Verb == "quit" || cmd.Verb == "q" {
			s.msgRouter[sid] = nil
			log.Printf("ending session %s", sid)
			return s.db.EndSession(sid)
		}
		send := s.msgRouter[sid]

		// TODO what is the implication of returning an error from this function?

		avatar, err := s.db.AvatarBySessionID(sid)
		if err != nil {
			return s.HandleError(send, err)
		}
		log.Printf("found avatar %#v", avatar)

		switch cmd.Verb {
		case "say":
			if err = s.HandleSay(avatar, cmd.Rest); err != nil {
				s.HandleError(func(_ *proto.ClientMessage) error { return nil }, err)
			}
		default:
			msg := &proto.ClientMessage{
				Type: proto.ClientMessage_WHISPER,
				Text: fmt.Sprintf("unknown verb: %s", cmd.Verb),
			}
			if err = send(msg); err != nil {
				s.HandleError(send, err)
			}
		}

		/*

			msg := &proto.ClientMessage{
				Type: proto.ClientMessage_OVERHEARD,
				Text: fmt.Sprintf("%s sent command %s with args %s",
					sid, cmd.Verb, cmd.Rest),
			}

			speaker := "ECHO"
			msg.Speaker = &speaker

			err = send(msg)
			if err != nil {
				log.Printf("failed to send %v to %s: %s", msg, sid, err)
			}
		*/
	}
}

func (s *gameWorldServer) Ping(ctx context.Context, _ *proto.SessionInfo) (*proto.Pong, error) {
	pong := &proto.Pong{
		When: "TODO",
	}

	return pong, nil
}

func (s *gameWorldServer) Messages(si *proto.SessionInfo, stream proto.GameWorld_MessagesServer) error {
	s.mu.Lock()
	s.msgRouter[si.SessionID] = stream.Send
	s.mu.Unlock()

	// TODO this is clearly bad but it works. I should refactor this so that messages are received on a channel.
	for {
	}
}

// TODO make sure the Foyer is created as part of initial setup / migration

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

	bedroom, err := s.db.BedroomBySessionID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find bedroom for %s: %w", sessionID, err)
	}

	err = s.db.MoveInto(*av, *bedroom)
	if err != nil {
		return nil, fmt.Errorf("failed to move %d into %d: %w", av.ID, bedroom.ID, err)
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

	bedroom, err := s.db.BedroomBySessionID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find bedroom for %s: %w", sessionID, err)
	}

	err = s.db.MoveInto(*av, *bedroom)
	if err != nil {
		return nil, fmt.Errorf("failed to move %d into %d: %w", av.ID, bedroom.ID, err)
	}

	si = &proto.SessionInfo{SessionID: sessionID}

	// TODO actually put them in world

	return
}

func (s *gameWorldServer) HandleSay(avatar *db.Object, msg string) error {
	name := avatar.Data["name"]
	if name == "" {
		// TODO determine this based on a hash or something
		name = "a mysterious figure"
	}

	heard, err := s.db.Earshot(*avatar)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	log.Printf("found %#v in earshot of %#v\n", heard, avatar)

	as, err := s.db.ActiveSessions()
	if err != nil {
		return err
	}

	sendErrs := []error{}

	for _, h := range heard {
		// TODO once we have a script engine, deliver the HEARS event
		for _, sess := range as {
			if sess.AccountID == h.OwnerID {
				cm := proto.ClientMessage{
					Type:    proto.ClientMessage_OVERHEARD,
					Text:    msg,
					Speaker: &name,
				}
				err = s.msgRouter[sess.ID](&cm)
				if err != nil {
					sendErrs = append(sendErrs, err)
				}
			}
		}
	}

	if len(sendErrs) > 0 {
		errMsg := "send errors: "
		for i, err := range sendErrs {
			errMsg += err.Error()
			if i < len(sendErrs)-1 {
				errMsg += ", "
			}
		}
		return errors.New(errMsg)
	}

	return nil
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
	// TODO at some point during startup clear out sessions
	err := _main()
	if err != nil {
		log.Fatal(err.Error())
	}
}
