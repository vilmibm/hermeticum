package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/vilmibm/hermeticum/proto"
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

func (s *gameWorldServer) Register(ctx context.Context, auth *proto.AuthInfo) (*proto.SessionInfo, error) {
	// TODO
	return nil, nil
}

// TODO other server functions

func main() {
	err := _main()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
