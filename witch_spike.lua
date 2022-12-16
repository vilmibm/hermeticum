-- This file contains some drafts on how a WITCH API could look in Lua.
-- Assumed in-scope functions:
--  hears(pattern, callback)
--    registers a callback called when an object in the same room as this one
--    SAYs something that matches pattern
--  sees(pattern, callback)
--    registers a callback called when an object in the same room as this one
--    EMOTEs something that matches pattern
--  says(message)
--    issues a SAY to the server that other objects in the same room will overhear
--  does(message)
--    issues an EMOTE to the server that other objects in the same room will see
--  room.says(message)
--    tells the server to have the room "say" something. this is useful when an
--    object wants something said in the third person.

-- Example 1: the nervous pasta
has = {
  name = "spaghetti",
  description = "a plate of spaghetti covered in a fresh pomodoro sauce"
}

hears("*eat*", function(msg)
  does("quivers nervously")
end)

sees("*slurp*", function(emote)
  does("inches away from " + sender.get("name"))
end)

-- Example 2: the slot machine
has = {
  name = "slot machine",
  description = "a vintage 1960s slot machine from Las Vegas"
}

provides("pull $this", function(args)
  math.randomseed(os.time())
  says("KA CHANK")
  one = math.random(0, 9)
  two = math.random(0, 9)
  three = math.random(0,9)
  say("you got" + string.format("%d %d %d", one, two, three))
end)

provides("whack $this", function(args)
  room.says(sender.get("name") + " hits the " + get("name") + " very hard ")
  say("CLATTLE KRAK CHAK CHUNK")
  say("you got 7 7 7")
end)

-- Example 3: vending machine
has = {
  name = "vending machine",
  description = "looks like the kind of thing you'd see in Tokyo"
}

provides("give $this $money", function(args)
  amount = args.get("money")
  -- TODO
end)
