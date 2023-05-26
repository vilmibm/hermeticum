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

Coming back to this after a long break, this new scheme with the exits table seems strictly worse than the tildemush approach. i don't like the amount of book keeping in the new approach--that's complexity that can lead to bugs. i think it's ultimately most elegant to just...let the exit exist in two rooms.

aside: i want to think through why exits shouldn't be on a room but it's a pretty quick answer. i don't want rooms that aren't world editable to be un-connectable to other things. if someone comes along and makes room A and then never comes back, it should be tunnel-able to from other rooms. i like the idea of people finding some cobwebbed room and then building a ladder up to it from somewhere.

so i'm going back to the tildemush approach. the next question is; is the exit maps a useful thing? couldn't the go handler just add a second, mirrored go handler? a handler that checks room directionality?

i've added a WITCH function, goes, which takes a direction and two rooms. this WITCH function adds two `go` verb handlers--one for the direction in the `goes` invocation and then one for the reverse. i like this more than the exit map, but it does mean that exits could compete each other. can't remember if that could happen in tildemush (was the exits map stored per exit? was that guarded?).

i think the competing is fine. i'm actually fine with it. i think that goes() could also add an invocation of `provides()` like this:

```lua
provides("use $this", function(args)
  move_sender("target_room")
end)
```

A nice idea is the ability for an exit to add flavor to an entity transitioning through it; for this I can maybe add:

```lua
goesAnd(east, "ossuary", "gallery", function(args)
  tellSender("the door squeals harshly as you move it but allows you passage")
end)
```

The movement is still generated.


so the problem with all of this is that the beahvior of the exit is
supposedly going to change based on how it is contained. however, we only
re-read the code when the code changes. we're trying to affect game state
as part of code compilation and that's uncool.

a user would create a new exit, put all the finishing touches on it, then move
it from their person to the room the exit originates from. WITCH would not
recompile and no containership would update.

i think the dream of goes(north, "pub") is donezo. i think the next best
thing is goes(north, "foyer", "pub"). on execution, the sender is checked;
if their container is the first room then they are moved to the second
room. both cases suffer from the double containment problem. that second
containment *cannot* be updated as part of WITCH compilation.

so we're back to two options:
- one way exits
- special creation semantics that handle something like double containership

in the interest of moving on--and of putting off special top level commands
that exist outside of WITCH--i want to do one way exits for now. this sucks
because all the flavor written for an exit has to be duplicated for its pair.

some other ideas because i can't let go. what of a variation on the exits map
where each exit stores a key in its has data about where it goes. this is no
better than a dynamic handler (it's worse) and does not help the double
containership problem.

give up and do one way exits.

well, now that i've given up and done one way exits the most pressing thing is to have object creation in client. it's just maddening to have to make test objects out-of-client. also revision tracking was one of the clunkiest things in the alpha, so i am eager to think that through anew.

## server beta

- [x] grpc server
- [x] session handling (opening/closing)
- [x] verb handling
- [ ] build out some more default rooms
- [x] DB: initial schema
- [ ] DB: sundry error handling
- [ ] DB/WITCH: locking objects
- [x] WITCH: initial setup
- [ ] WITCH: ability to send verbs outward
- [ ] WITCH: transitive verb support
- [ ] WITCH: provides function
- [ ] WITCH: movement stuff (teleport, move)
- [ ] WITCH: bidirectional has() support
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
