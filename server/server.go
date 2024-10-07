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

/*

I want dig to exist. what should happen?

/dig north
There is already a room in that direction.
/go north
You are in the empty field.
/dig north
You have breathed life into a new object! Its true name is 12345
/edit 12345

($EDITOR opens with witch code, user edits)

"edit" will have to be a specially noted client command that ultimately sends a cmd to the server like:

Cmd{verb: "edit", "rest": newcode}

perhaps first it could send Cmd{verb: "lock", "rest": objid}

and the edit handler does the update and unlock

so to start, let's support 'dig'.
*/

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

	// TODO check lock

	if target.Perms.Exec == db.PermOwner && sender.ID != target.OwnerID {
		return nil
	}

	s.scriptsMutex.RLock()
	sc, ok := s.scripts[target.ID]
	s.scriptsMutex.RUnlock()
	var err error

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
	// gid := pai.ucred.Gid TODO staff powers

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

	log.Printf("uid %d connected", uid)

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
		affected, err := avatar.Earshot(s.db)
		if err != nil {
			log.Printf("error trying to inform others about a derez: %s", err.Error())
			return
		}
		s.db.Derez(uid)

		for _, obj := range affected {
			if obj.Avatar {
				aio, ok := s.sessions[uint32(obj.OwnerID)]
				if ok {
					aname, ok := avatar.Data["name"]
					if !ok {
						aname = "amorphous entity"
					}
					msg := "slowly fades out of existence"
					aio.outbound <- &proto.WorldEvent{
						Type:   proto.WorldEvent_EMOTE,
						Source: &aname,
						Text:   &msg,
					}
				}
			}
		}
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
			log.Printf("cmd %s %s from uid %d", cmd.Verb, cmd.Rest, uid)
			switch cmd.Verb {
			case "quit":
				uio.done <- true
			case "dig":
				go func() {
					err = s.handleDig(*avatar, cmd)
					if err != nil {
						uio.errs <- err
					}
				}()
			default:
				go func() {
					err = s.handleCmd(*avatar, cmd)
					if err != nil {
						uio.errs <- err
					}
				}()
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

func (s *gameWorldServer) handleDig(avatar db.Object, cmd *proto.Command) error {
	uid := uint32(avatar.OwnerID)
	heading := cmd.Rest
	if !witch.ValidDirection(heading) {
		msg := fmt.Sprintf("sorry, %s is not a valid heading. valid headings are: %v", heading,
			witch.Directions())

		s.sessions[uid].outbound <- &proto.WorldEvent{
			Type: proto.WorldEvent_PRINT,
			Text: &msg,
		}
	}
	dir := witch.NormalizeDirection(heading)

	currentRoom, err := avatar.Container(s.db)
	if err != nil {
		return err
	}

	room := db.NewRoom(uid)
	if err = room.Save(s.db); err != nil {
		return err
	}

	desc := "a simple wooden gate on a hinge"
	name := "small gate"
	log.Printf("%#v", dir)
	if dir.IsVertical() {
		desc = "a basic wooden ladder. it's a little rickety."
		name = "ladder"
	}

	door := db.NewObject(uid)
	door.SetData("name", name)
	door.SetData("description", desc)
	door.AppendScript(fmt.Sprintf("goes(%s, %d)", heading, room.ID))
	if err = door.Save(s.db); err != nil {
		return err
	}

	revDoor := db.NewObject(uid)
	revDoor.SetData("name", name)
	revDoor.SetData("description", desc)
	revDoor.AppendScript(fmt.Sprintf("goes(%s, %d)",
		dir.Reverse().Human(), currentRoom.ID))

	if err = revDoor.Save(s.db); err != nil {
		return err
	}

	err = s.db.MoveInto(*door, *currentRoom)
	if err != nil {
		return err
	}

	err = s.db.MoveInto(*revDoor, *room)
	if err != nil {
		return err
	}

	// TODO inform user about some things?
	return nil
}

func (s *gameWorldServer) handleCmd(avatar db.Object, cmd *proto.Command) error {
	affected, err := avatar.Earshot(s.db)
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
