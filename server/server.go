package server

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
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

type ServeOpts struct {
	Port int
}

type ServerAuthCredentials struct {
	credentials.TransportCredentials
}

type PeerAuthInfo struct {
	credentials.CommonAuthInfo
	ucred *unix.Ucred
}

func (PeerAuthInfo) AuthType() string {
	return "TODO"
}

func readCreds(c net.Conn) (*unix.Ucred, error) {
	// From https://blog.jbowen.dev/2019/09/using-so_peercred-in-go/
	var cred *unix.Ucred

	// net.Conn is an interface. Expect only *net.UnixConn types
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("unexpected socket type")
	}

	// Fetches raw network connection from UnixConn
	raw, err := uc.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("error opening raw connection: %s", err)
	}

	// The raw.Control() callback does not return an error directly.
	// In order to capture errors, we wrap already defined variable
	// 'err' within the closure. 'err2' is then the error returned
	// by Control() itself.
	err2 := raw.Control(func(fd uintptr) {
		cred, err = unix.GetsockoptUcred(int(fd),
			unix.SOL_SOCKET,
			unix.SO_PEERCRED)
	})

	if err != nil {
		return nil, fmt.Errorf("GetsockoptUcred() error: %s", err)
	}

	if err2 != nil {
		return nil, fmt.Errorf("Control() error: %s", err2)
	}

	return cred, nil
}

func (*ServerAuthCredentials) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	pai := PeerAuthInfo{}
	ucred, err := readCreds(conn)
	if err != nil {
		return conn, pai, err
	}
	pai.ucred = ucred

	return conn, pai, nil
}

func Serve(opts ServeOpts) error {
	gs := grpc.NewServer(grpc.Creds(&ServerAuthCredentials{}))

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", opts.Port))
	if err != nil {
		return err
	}
	s, err := newServer()
	if err != nil {
		return err
	}

	proto.RegisterGameWorldServer(gs, s)
	gs.Serve(l)

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
	// TODO switch to a little pg
	// TODO audit and clean up all of this
	db, err := db.NewDB()
	if err != nil {
		return nil, err
	}

	reset := flag.Bool("reset", false, "fully reset the database to its initial state")
	flag.Parse()

	if *reset {
		if err = db.Erase(); err != nil {
			return nil, fmt.Errorf("failed to reset database: %w", err)
		}
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
	log.Printf("VH %s %s %d %d", verb, rest, sender.ID, target.ID)

	s.scriptsMutex.RLock()
	sc, ok := s.scripts[target.ID]
	s.scriptsMutex.RUnlock()
	var err error

	getSend := func(sid string) func(*proto.ClientMessage) error {
		return s.msgRouter[sid]
	}

	if !ok || sc == nil {
		if sc, err = witch.NewScriptContext(s.db, getSend); err != nil {
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

		for _, obj := range affected {
			log.Printf("%s heard %s from %d", obj.Data["name"], cmd.Verb, avatar.ID)
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

	// TODO this is clearly bad but it works. I should refactor this so that
	// messages are received on a channel.
	for {
	}
}

func (s *gameWorldServer) Register(ctx context.Context, auth *proto.AuthInfo) (si *proto.SessionInfo, err error) {
	// TODO delete this
	/*
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
	*/

	return
}

func (s *gameWorldServer) Login(ctx context.Context, auth *proto.AuthInfo) (si *proto.SessionInfo, err error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		// TODO
		panic("did not get peer from context")
	}
	pai, ok := p.AuthInfo.(PeerAuthInfo)
	if !ok {
		// TODO
		panic("typecast failed")
	}
	fmt.Println(pai)

	/*
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
	*/

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
