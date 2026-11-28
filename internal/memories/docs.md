# Noridoc: memories

Path: @/internal/memories

### Overview

- Owns `memories.json`, the owner-only file of what Ziggy remembers about the household between conversations: short sentences such as "Emma has soccer on Tuesdays."
- Separate from @/internal/config because `config.json` is served to every browser without authentication; memories are private household data and are only returned by the admin memory endpoints.
- Distinct from the agent's in-process conversation history (`memory.go` in @/internal/agent), which lasts five minutes and is lost on restart.

### How it fits into the larger codebase

```
 cmd/server openStores(CONFIG_PATH)
   `-- memories.Open(PathFor(config.json))  -> memories.json beside it
            |
            v
 internal/server  (Options.Memories, required: New fails without it)
   |-- memories.go        GET/POST /api/agent/memories, PUT/DELETE /api/agent/memories/{id}
   |-- agent.Options.Memories   List() read into the instructions every turn
   `-- tools.Deps.Memories      remember / update_memory / forget tools
```

- Opened once at startup next to `config.json` (in Docker, `/data/memories.json`). A corrupt file fails startup rather than being replaced, so memories are never silently lost.
- Ziggy changes memories through the tools in @/internal/agent/tools; the household changes them from the admin Memories tab (@/web/src/admin/MemoriesPanel.tsx). Both apply on Ziggy's next turn, because @/internal/agent re-reads `List()` when building each turn's instructions.

### Core Implementation

- `Memory{ID, Text, Created, Updated}`; ids are UUIDs, times UTC. `List()` returns a copy, oldest first, and never nil (so the admin endpoint sends `[]`, not `null`, before anything is saved).
- `Add`, `Update(id, text)` and `Delete(id)` go through `update`, which applies the change to a copy under the store's mutex, writes it (temp file in the same directory, `0600`, fsync, rename) and only then swaps it in, so a failed write changes nothing.
- Text has all whitespace collapsed to single spaces (`strings.Fields`), so a memory can't contain a line break: memories are listed one per line in Ziggy's instructions and must not be able to start lines of their own there. Empty or longer than `MaxTextLen` (300 characters) is `ErrInvalid`. `Add` past `MaxMemories` (150) is `ErrFull`; updates and deletes still work when full. Unknown ids are `ErrNotFound`.

### Things to Know

- The error messages are written as sentences Ziggy can relay ("memory is full (150 memories)"); the server capitalises them for admin. `ErrFull` deliberately doesn't suggest forgetting something, so Ziggy never deletes a household memory on his own just to make room.
- Every memory is sent to the model on every turn, so the caps also bound prompt size (at most about 51,000 characters with ids).
- What Ziggy should and shouldn't remember (no sensitive details, no duplicates, saved silently unless asked) is in the agent's persona, not enforced here.

Created and maintained by Nori.
