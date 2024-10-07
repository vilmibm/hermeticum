blowing off the dust

- [X] single binary
- [X] add in cobra
- [X] peercred `s := grpc.NewServer(grpc.Creds(&ServerAuthCredentials{}))` `type ServerAuthCredentials struct { credentials.TransportCredentials}`
- [X] gut client and remake as basic as possible (maybe still with tview for two pane)
- [O] trim down protobuff (draw up new api)
  - bunch of raw notes on this in the protobuff definition file
  - [X] update the definition
  - [X] pseudocode the handling of new Commands
  - [X] try to think of a better name than Commands...
  - [X] implement the pseudo code
- [X] understand why i did `getSend`
  - i never did. i just did something far more straightforward and it was fine. i was struggling the last time i worked on this...
- [X] fix script contexts' ability to send events to clients
- [X] refresh bare bones client
- [ ] add create
- [ ] add dig
