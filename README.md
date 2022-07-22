hermeticum is the next version of [tildemush](https://github.com/vilmibm/tildemush): a full rewrite of the alpha release.

there are two major differences: Go instead of Python and Lua instead of a from-scratch scripting language for game objects.

Otherwise, this is a pretty faithful implementation of what tildemush was planned to be (and mostly implemented in the alpha).

## but why

the alpha version of tildemush does work! you can script objects and make rooms and do all sorts of things. Unfortunately, it:

- gets laggy with any real number of users
- has memory leaks
- consists of very poorly abstracted spaghetti code
- has a half-implemented client
- is extremely hard to add to (I gave up trying to do scheduled tasks)
- has a very brittle, very fragile, 100% hacks scripting language for game objects
- has a very inelegant and inefficient system for handling revisions to game objects code

I feel very strongly that a total rewrite is necessary. I also feel very strongly that Go is a better choice for this kind of application than Python.

## new technical stuff

- API is grpc/protobuff based
- client uses `tview` which I find more pleasant to work with that `urwid`

## the name though

the name "tildemush" has never quite sat right with me. I don't know why. I'm a
lot more pleased with `hermeticum`, which describes what it is I'm really
inspired by when it comes to MU* engines: the spaces a mind can create to store
wisdom. I like the idea of mapping these mental spaces into a computer.

## docs

I haven't moved over any design docs or notes or anything like that. Refer to the tildemush repo for that kind of stuff.

to regenerate the API code: `protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/hermeticum.proto`
