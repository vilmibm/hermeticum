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
	"github.com/vilmibm/hermeticum/server/witch"
	"google.golang.org/grpc"
)

var (
	tls      = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	certFile = flag.String("cert_file", "", "The TLS cert file")
	keyFile  = flag.String("key_file", "", "The TLS key file")
	port     = flag.Int("port", 6666, "The server port")
)

/*
	I'm going to take a much simpler approach to scripts than I did in tildemush: objects just get one text column with no revision tracking. to avoid re-parsing scripts per verb check (as every overheard verb has to be checked against an object's script every verb utterance) i want an in-memory cache of lua states. i'm not actually sure if the goroutine unsafety is a problem for that. if it is, i can put goroutines in memory and send them verbs over channels. annoying, but should work if i have to. going to start with a naive map of object ids to scripts, re-parsing them if they get edited and updating the cache.

	The cache will grow without bound as users enter rooms with objects. they ought to be garbage collected. i can do that in a goroutine though (check DB for objects in rooms with no players). One complication will be when I have "cron" support for objects. they will need to be "live" (ie, their scripts executable) in order to do their periodic tasks.

	An idea I just had for a cron system: respond to a "tick" verb. at server start, once all in-world objects get parsed, start emitting "tick" events from a for loop in a goroutine in rooms with objects. this can be optimized for having a way to flag periodic-able objects so they don't get the verb if they wouldn't respond.

*/

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

	db             db.DB
	msgRouterMutex sync.Mutex
	msgRouter      map[string]func(*proto.ClientMessage) error
	Gateway        *witch.Gateway
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
	/*
		So right here is a problem: we are definitely making >1 LuaState per
		goroutine. Top priority before anything else is getting a goroutine made
		for the script contexts
	*/
	s.scriptsMutex.RLock()
	sc, ok := s.scripts[target.ID]
	s.scriptsMutex.RUnlock()

	if !ok || sc.NeedsRefresh(target) {
		sc, err := witch.NewScriptContext(target)
		if err != nil {
			return err
		}

		s.scriptsMutex.Lock()
		s.scripts[target.ID] = sc
		s.scriptsMutex.Unlock()
	}

	return sc.Handle(verb, rest, sender, target)
}

func (s *gameWorldServer) HandleCmd(verb, rest string, sender *db.Object) {
	// TODO
}

/*
	what's the flow for when i'm at a computer and type /say hi ?

	- server gets "SAY hi" from vilmibm
	- server gets all objects in earshot (including vilmibm's avatar)
	- for each object:
		- call whatever handler it has for "hears"

	and then that's it, right? over in witch land:

	- hears handler for an avatar has:

			tellMe(sender.get("name") + " says " + msg)

	- tellMe somehow calls a method on the gameWorldServer that can look up a
		session ID and thus use the msgRouter to send a message. I'm going to sleep
		on this so I can think about the right way to structure those dependencies.

*/

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

		affected, err := s.db.Earshot(*avatar)

		for _, o := range affected {
			err = s.Gateway.VerbHandler(cmd.Verb, cmd.Rest, *avatar, o)
		}

		s.HandleCmd(cmd.Verb, cmd.Rest, avatar)

		/*

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

		*/

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
	s.msgRouterMutex.Lock()
	s.msgRouter[si.SessionID] = stream.Send
	s.msgRouterMutex.Unlock()

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

	/*
		bedroom, err := s.db.BedroomBySessionID(sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to find bedroom for %s: %w", sessionID, err)
		}

		err = s.db.MoveInto(*av, *bedroom)
		if err != nil {
			return nil, fmt.Errorf("failed to move %d into %d: %w", av.ID, bedroom.ID, err)
		}
	*/

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

	bedroom, err := s.db.BedroomBySessionID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find bedroom for %s: %w", sessionID, err)
	}

	err = s.db.MoveInto(*av, *bedroom)
	if err != nil {
		return nil, fmt.Errorf("failed to move %d into %d: %w", av.ID, bedroom.ID, err)
	}

	si = &proto.SessionInfo{SessionID: sessionID}

	return
}

func (s *gameWorldServer) HandleSay(sender *db.Object, msg string) error {
	name := sender.Data["name"]
	if name == "" {
		// TODO determine this based on a hash or something
		name = "a mysterious figure"
	}

	heard, err := s.db.Earshot(*sender)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	log.Printf("found %#v in earshot of %#v\n", heard, sender)

	as, err := s.db.ActiveSessions()
	if err != nil {
		return err
	}

	sendErrs := []error{}

	// TODO figure out pointer shit

	for _, h := range heard {
		s.Gateway.VerbHandler("hears", msg, sender, &h)
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
