package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

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
	proto.RegisterGameWorldServer(grpcServer, newServer())
	grpcServer.Serve(l)

	return nil

}

type gameWorldServer struct {
	proto.UnimplementedGameWorldServer
}

func newServer() *gameWorldServer {
	s := &gameWorldServer{}
	return s
}

func (s *gameWorldServer) Ping(ctx context.Context, _ *proto.SessionInfo) (*proto.Pong, error) {
	pong := &proto.Pong{
		When: "TODO",
	}

	return pong, nil
}

func (s *gameWorldServer) Messages(si *proto.SessionInfo, stream proto.GameWorld_MessagesServer) error {
	for x := 0; x < 100; x++ {
		msg := &proto.ClientMessage{}
		speaker := "snoozy"
		msg.Speaker = &speaker
		msg.Type = proto.ClientMessage_WHISPER
		msg.Text = fmt.Sprintf("hi this is message %d. by the way i am a horse. neigh neigh neigh neigh neigh neigh neigh neigh neigh neigh neigh neigh", x)
		stream.Send(msg)
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func (s *gameWorldServer) Register(ctx context.Context, auth *proto.AuthInfo) (si *proto.SessionInfo, err error) {
	err = db.CreateAccount(auth.Username, auth.Password)
	if err != nil {
		return nil, err
	}

	var sessionID string
	sessionID, err = db.StartSession(auth.Username)
	if err != nil {
		return nil, err
	}

	si = &proto.SessionInfo{SessionID: sessionID}

	return
}

func (s *gameWorldServer) Login(ctx context.Context, auth *proto.AuthInfo) (si *proto.SessionInfo, err error) {
	err = db.ValidateCredentials(auth.Username, auth.Password)
	if err != nil {
		return
	}

	var sessionID string
	sessionID, err = db.StartSession(auth.Username)
	if err != nil {
		return nil, err
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
