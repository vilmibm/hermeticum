package main

import (
	"context"
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

		// TODO find the user who ran action via SessionInfo
		// TODO get area of effect, which should include the sender
		// TODO dispatch the command to each affected object
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

func (s *gameWorldServer) Register(ctx context.Context, auth *proto.AuthInfo) (si *proto.SessionInfo, err error) {
	var account *db.Account
	account, err = s.db.CreateAccount(auth.Username, auth.Password)
	if err != nil {
		return
	}

	var avatar *db.Object
	avatar, err = s.db.CreateAvatar(account)
	if err != nil {
		return
	}

	log.Printf("created %#v for %s", avatar, account.Name)

	// TODO create avatar object
	// TODO create bedroom object
	// TODO put avatar in bedroom
	// TODO send room info, avatar info to client (need to figure this out and update proto)

	var sessionID string
	sessionID, err = s.db.StartSession(*account)
	if err != nil {
		return nil, err
	}
	log.Printf("started session for %s", account.Name)

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

	si = &proto.SessionInfo{SessionID: sessionID}

	return
}

// TODO other server functions

func main() {
	err := _main()
	if err != nil {
		log.Fatal(err.Error())
	}
}
