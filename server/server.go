package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"strconv"
	"sync"
	"time"

	"github.com/vilmibm/hermeticum/proto"
	"github.com/vilmibm/hermeticum/server/db"
	"github.com/vilmibm/hermeticum/server/witch"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

type ServeOpts struct {
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

const sockAddr = "/tmp/hermeticum.sock"

func Serve(opts ServeOpts) error {
	os.Remove(sockAddr)

	l, err := net.Listen("unix", sockAddr)
	if err != nil {
		return err
	}
	defer l.Close()

	os.Chmod(sockAddr, 0777) // frisson

	gs := grpc.NewServer(grpc.Creds(&ServerAuthCredentials{}))
	s, err := newServer()
	if err != nil {
		return err
	}

	proto.RegisterGameWorldServer(gs, s)
	log.Println("i'm starting")
	log.Printf("sock address: %s", sockAddr)
	gs.Serve(l)

	return nil
}

type gameWorldServer struct {
	proto.UnimplementedGameWorldServer

	db           *db.DB
	sessions     map[uint32]*userIO
	sessionMutex sync.Mutex
	scripts      map[int]*witch.ScriptContext
	scriptsMutex sync.RWMutex
}

func newServer() (*gameWorldServer, error) {
	db, err := db.NewDB()
	if err != nil {
		return nil, err
	}
	if err = db.Ensure(); err != nil {
		return nil, fmt.Errorf("failed to ensure default entities: %w", err)
	}

	if err = db.GhostBust(); err != nil {
		return nil, fmt.Errorf("could not clear sessions: %w", err)
	}

	s := &gameWorldServer{
		sessions:     make(map[uint32]*userIO),
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

	/*
		I am at a loss why I did this getSend thing. I feel like I arrived at it after some amount of trial/error/debugging/frustration.

		Ideally script contexts just have a clientSend function that accepts a WorldEvent. I will just start switching to that and see what goes wrong.
	*/

	clientSend := func(uid uint32, ev *proto.WorldEvent) {
		if uio, ok := s.sessions[uid]; ok {
			uio.outbound <- ev
		} else {
			// TODO log this
		}
	}

	if !ok || sc == nil {
		if sc, err = witch.NewScriptContext(s.db, clientSend); err != nil {
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

type userIO struct {
	inbound  chan *proto.Command
	outbound chan *proto.WorldEvent
	errs     chan error
	done     chan bool
}

func (s *gameWorldServer) ClientInput(stream proto.GameWorld_ClientInputServer) error {
	ctx := stream.Context()
	p, ok := peer.FromContext(ctx)
	if !ok {
		return errors.New("failed to get peer information from context")
	}
	pai, ok := p.AuthInfo.(PeerAuthInfo)
	if !ok {
		return errors.New("failed to cast PeerAuthInfo")
	}
	uid := pai.ucred.Uid
	gid := pai.ucred.Gid

	fmt.Println(uid, gid)

	if _, ok := s.sessions[uid]; ok {
		return fmt.Errorf("existing session for %d", uid)
	}

	u, err := user.LookupId(fmt.Sprintf("%d", uid))
	if err != nil {
		return fmt.Errorf("could not find user for uid %d: %w", uid, err)
	}

	avatar, err := s.db.GreateAvatar(uid, u.Username)
	if err != nil {
		return fmt.Errorf("failed to get or create avatar for %d: %w", uid, err)
	}

	fmt.Println(avatar)

	uio := &userIO{
		inbound:  make(chan *proto.Command),
		outbound: make(chan *proto.WorldEvent),
		errs:     make(chan error, 1),
		done:     make(chan bool, 1),
	}

	rootu, err := user.Lookup("root")
	if err != nil {
		return err
	}
	ruid, err := strconv.Atoi(rootu.Uid)
	if err != nil {
		return err
	}

	rootuid := uint32(ruid)

	s.sessionMutex.Lock()
	s.sessions[uid] = uio
	s.sessionMutex.Unlock()

	defer func() {
		log.Printf("ending session for %d", uid)
		s.sessionMutex.Lock()
		delete(s.sessions, uid)
		s.sessionMutex.Unlock()
		s.db.Derez(uid)
		// TODO send message to earshot about departure
	}()

	go func() {
		for {
			if cmd, err := stream.Recv(); err != nil {
				uio.errs <- err
				uio.done <- true
			} else {
				uio.inbound <- cmd
			}
		}
	}()

	foyer, err := s.db.GetObject(rootuid, "foyer")
	if err != nil {
		return fmt.Errorf("failed to find foyer: %w", err)
	}

	if err = s.db.MoveInto(*avatar, *foyer); err != nil {
		return fmt.Errorf("failed to move %d into %d: %w", avatar.ID, foyer.ID, err)
	}

	for {
		select {
		case cmd := <-uio.inbound:
			log.Printf("verb %s from uid %d", cmd.Verb, uid)
			if cmd.Verb == "quit" || cmd.Verb == "q" {
				uio.done <- true
			} else {
				err := s.handleCmd(*avatar, cmd)
				if err != nil {
					uio.errs <- err
				}
			}
		case ev := <-uio.outbound:
			if err := stream.Send(ev); err != nil {
				uio.errs <- err
			}
		case err := <-uio.errs:
			log.Printf("error in stream for %d: %s", uid, err.Error())
		case <-uio.done:
			return nil
		}
	}
}

func (s *gameWorldServer) handleCmd(avatar db.Object, cmd *proto.Command) error {
	affected, err := s.db.Earshot(avatar)
	if err != nil {
		return err
	}

	for _, obj := range affected {
		log.Printf("%s heard %s from %d", obj.Data["name"], cmd.Verb, avatar.ID)
	}

	for _, o := range affected {
		if err = s.verbHandler(cmd.Verb, cmd.Rest, avatar, o); err != nil {
			log.Printf("error handling verb %s for object %d: %s", cmd.Verb, o.ID, err)
		}
	}

	return nil
}

func (s *gameWorldServer) Ping(ctx context.Context, _ *proto.PingMsg) (*proto.Pong, error) {
	// TODO compute delta
	pong := &proto.Pong{
		Delta: "TODO",
		When:  fmt.Sprintf("%d", time.Now().Unix()),
	}

	return pong, nil
}
