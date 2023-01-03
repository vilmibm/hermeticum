# hermeticum roadmap

_being a rather unsorted, evolving, and utterly incomplete braindump of tasks for hermeticum_

for the next push, i see two things: editing WITCH scripts in-client and inter-room movement. I think I'd rather work on inter-room movement until not being able to edit WTICH scripts in-client becomes unbearable.

In tildemush, inter-room movement worked like this:

- there were six directions:
  - north
  - south
  - east
  - west
  - above/up
  - below/down
- rooms contained "exit" objects
- exit objects are defined as objects with an `exit` key in their data map
- the exit map gets two keys added:
  - one for the room in which the exit was created pointing to target room with given direction
  - one for the target room with reverse of given direction
- the exit object is then added to _both_ the current and target room
- when a go command is observed, the server checks the current room for exit objects and finds the one with the given direction and moves the sender to the room it targets

I don't like the exit simultaneously existing in two rooms at once or the special handling at the server level.

I can't actually remember how I ended up with the previous exit design. I think I wanted exits to be owned objects, but in retrospect, that leads to some clumsy code and situations.

I want exits to be game objects because they should be scriptable. if they are game objects, they have to exist somewhere in the game. They then have to be contained and I think should only be contained in one room. Thus, there is the problem of how to know what exits a room has if one of its exits is in its adjoining room. My proposal for this is as such:

- add an exits table with columns
  - start_room
  - end_room
  - direction
  - exitID
- when parsing an object, ensure it matches the exits table (keep track of what `go` handler it has)
- when checking a room for exits, search exits table for rows in exits where start_room or end_room match the room

This means that to edit an exit you would need to be in the room where the exit actually exists (start room). i could sugarize this by ensuring that exits technically contained in another room be included in the Earshot list. this would make them behave as if they were contained in two places but wouldn't actually violate physics.

It's tempting to have the `exits` map because of its simplicity, but it actually reduces scriptability. by forcing the WITCH surface area of exits to be a "go" handler we get a chance as the system to hook into exit creation and a chance as a user to hook into people moving through exits.

TODO draft something new

## server beta

- [x] grpc server
- [x] session handling (opening/closing)
- [x] verb handling
- [x] WITCH: initial setup
- [x] DB: initial schema
- [ ] DB: sundry error handling
- [ ] build out some more default rooms
- [ ] WITCH: ability to send verbs outward
- [ ] WITCH: transitive verb support
- [ ] WITCH: provides function
- [ ] WITCH: movement stuff (teleport, move)
- [ ] VERBS: create
- [ ] VERBS: announce (for gods)
- [ ] VERBS: movement
- [ ] VERBS: inventory
  - [ ] get
  - [ ] drop
  - [ ] view inventory
- [ ] VERBS: script editing
- [ ] VERBS: look
- [ ] VERBS: examine
- [ ] password hashing
- [ ] encrypted connection
- [ ] cron system
- [ ] room mapping
- [ ] global chat
- [ ] loudness system

## client beta

- [x] basic tview app
- [x] registration
- [x] login
- [ ] sundry error handling
- [ ] encrypted connection
- [ ] ping/pong tracking for server health report
- [ ] room mapping
- [ ] global chat
- [ ] details pane (see: examine command)
- [ ] script editing
