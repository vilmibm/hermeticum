-- This file contains some drafts on how a WITCH API could look in Lua.
-- Assumed in-scope functions:
--  has(data)
--    tells the server what data this object has. If this object is live (ie,
--    in world) when opened for editing this invocation will match the current
--    state of the data in the database. Anything can go in here and all keys
--    are optional.
--  allows(permissions)
--    tells the server what this object's permissions are. the permissions are
--    read, write, carry, and execute. permissions can have value of either
--    "owner" or "world". if unspecified, a permission is set to "owner".
--  hears(pattern, callback)
--    registers a callback called when an object in the same room as this one
--    SAYs something that matches pattern
--  sees(pattern, callback)
--    registers a callback called when an object in the same room as this one
--    EMOTEs something that matches pattern
--  provides(verb_pattern, callback)
--    registers a callback called when an object in the same room as this one runs "/verb_pattern"
--  says(message)
--    issues a SAY to the server that other objects in the same room will overhear
--  does(message)
--    issues an EMOTE to the server that other objects in the same room will see
--  room.says(message)
--    tells the server to have the room "say" something. this is useful when an
--    object wants something said in the third person.
--  random(min, max)
--    returns a random integer between min and max. This is provided so callers
--    do not have to worry about seeding randomness themselves.
--  create(name, code)
--    returns a new object with the given name and Lua code. owner and permissions are transferred from calling object
--  drop(object)
--    drop an object on the ground of the current room
--  tell_sender(msg)
--    issues a WHISPER to whoever triggered a callback
--  teleport_sender(room_id)
--    teleports whoever triggered a callback to the specified room
--  move_sender(direction)
--    teleports whoever triggered a callback in the specified direction
--
--  Callbacks are passed "args," a table which contains the utterance that
--  triggered the handler. this table can be accessed in a few ways:
-- 
--  args.get("thing")
--    returns whatever string occupied a placeholder called $thing. for example, in this provides:
--
--    provides("give $thing", function(args) end)
--
--    args.get("thing") has whatever space delimited value came after "give".
--
--  args.contains("some substring")
--    returns true if "some substring" is found when all of the args are joined
--    together in one string
--

-- Example 1: the nervous pasta
has({
  name = "spaghetti",
  description = "a plate of spaghetti covered in a fresh pomodoro sauce"
})

allows({
  read = "world",
  write = "owner"
  carry = "owner",
  execute = "world",
})

hears("*eat*", function(msg)
  does("quivers nervously")
end)

sees("*slurp*", function(emote)
  does("inches away from " + sender.get("name"))
end)

-- Example 1a: the nervous pasta written pedantically
-- This example just demonstrates the syntactic sugar of "hears" and "sees"
has({
  name = "spaghetti",
  description = "a plate of spaghetti covered in a fresh pomodoro sauce"
})

allows({
  read = "world",
  write = "owner"
  carry = "owner",
  execute = "world",
})

provides("say", function(args)
  if args.contain("eat") then
    does("quivers nervously")
  end
end)

provides("emote", function(args)
  if args.contain("slurp") then
    does("inches away from " + sender.get("name"))
  end
end)

-- Example 2: the slot machine
has({
  name = "slot machine",
  description = "a vintage 1960s slot machine from Las Vegas"
})

allows({
  read = "world",
  write = "owner"
  carry = "owner",
  execute = "world",
})

provides("pull $this", function(args)
  says("KA CHANK")
  one   = random(0, 9)
  two   = random(0, 9)
  three = random(0, 9)
  say("you got" + string.format("%d %d %d", one, two, three))
end)

provides("whack $this", function(args)
  room.says(sender.get("name") + " hits the " + get("name") + " very hard ")
  say("CLATTLE KRAK CHAK CHUNK")
  say("you got 7 7 7")
end)

-- Example 3: vending machine
has({
  name = "vending machine",
  description = "looks like the kind of thing you'd see in Tokyo",
  money = 0
})

allows({
  read = "world",
  write = "owner"
  carry = "owner",
  execute = "world",
})

provides("give $this $money $unit", function(args)
  amount = tonumber(args.get("money")) or 0
  unit = args.get("unit")
  if unit != "yen" then
    say("i only take yen sorry")
    return
  end

  set("money", get("money") + amount)
  if get("money") > 100 then
    room.says("the " + get("name") + " clatters a bit then drops a pocari sweat on the floor")
    set("money", 0)
    bottle = create("a bottle of pocari sweat", "")
    drop(bottle)
  else
    say("i need more money")
  end
end)

-- Example 3: a rusty door
has({
  name = "rusty metal door"
  description = "it's almost fully consumed by rust but still heavy and solid feeling"
})

allows({
  read = "world",
  write = "owner"
  carry = "owner",
  execute = "world",
})

-- option 1: fully manual

provides("go east", function(args)
  if sender.where = "gallery" then
    move_sender("ossuary")
  end
end)

provides("go west", function(args)
  if sender.where = "ossuary" then
    move_sender("gallery")
  end
end)

-- option 2: magical helper

-- automatically creates the two `go` handlers above
goes("east", "gallery", "ossuary")
