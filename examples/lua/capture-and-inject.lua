-- Watch a unit run, then change its mind — in one debug session.
--
-- Everything here goes through one session the engine holds for the life of the
-- script. That is the whole difference from the bindings this replaces: a debug
-- session lives in an ABAP roll area, and a client that opens a connection per
-- call cannot get back to it, so the old bindings returned happily and referred
-- to nothing.
--
--   vsp lua examples/lua/capture-and-inject.lua
--
-- The workload comes from somewhere else — a session cannot catch itself. Run
-- this, then from another terminal:
--   vsp rfc call ZADT_DEBUG_LOOP '{}'
--
-- One catch per session: an ADT session runs one debug session, and after a
-- detach the next attach is refused. For another round, run the script again.

local TARGET, LINE = "ZADT_DEBUG_LOOP", 9

clearBreakpoints()
local bps = setBreakpoint(TARGET, LINE)
print(string.format("anchored at %s:%d — %s", TARGET, LINE, bps[1].id))

print("\nwaiting for somebody to run it…")
local who, err = listen(120)
if not who then print("nobody ran it: " .. tostring(err)); return end
print(string.format("caught %s at %s/%s:%d\n", who.user, who.program, who.include, who.line))

-- Watch the statement that reads the database, and see what it read.
local at = stepOver()
local read = locals()
print(string.format("line %d — the database said LV_LOW = %s", at.line, read.LV_LOW.value))

-- Now decide otherwise. Reaching a state by arranging for the system to produce
-- it is usually the hard part of a test; this skips it.
setVariable("LV_LOW", "4200")
print("injected                      LV_LOW = 4200")

-- And watch the rest of the unit compute with it.
local history = record(20, true)
for _, r in ipairs(history) do
  local vals = {}
  for name, v in pairs(r.vars or {}) do vals[#vals+1] = name .. "=" .. v end
  table.sort(vals)
  print(string.format("  %2d %-5s line %-4d %s", r.seq, r.kind, r.line, table.concat(vals, "  ")))
end

detach()
print("\nthe unit ran to completion with a value it never read")
