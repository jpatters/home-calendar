import { useEffect, useState } from "react";
import type { Memory } from "../types";
import { addMemory, deleteMemory, listMemories, updateMemory } from "../api";

// What Ziggy remembers. It lives outside the config (which every display can
// read), so each change is sent to its own endpoint straight away rather than
// with the page's Save.
export default function MemoriesPanel() {
  const [memories, setMemories] = useState<Memory[] | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [newText, setNewText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    listMemories().then(
      (list) => {
        if (!cancelled) setMemories(list);
      },
      () => {
        if (!cancelled) setLoadFailed(true);
      },
    );
    return () => {
      cancelled = true;
    };
  }, []);

  const run = async (request: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await request();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const add = () =>
    run(async () => {
      const m = await addMemory(newText);
      setMemories((list) => [...(list ?? []), m]);
      setNewText("");
    });

  const save = (id: string) =>
    run(async () => {
      const m = await updateMemory(id, drafts[id]);
      setMemories((list) => (list ?? []).map((x) => (x.id === id ? m : x)));
      setDrafts(({ [id]: _, ...rest }) => rest);
    });

  const remove = (id: string) =>
    run(async () => {
      await deleteMemory(id);
      setMemories((list) => (list ?? []).filter((x) => x.id !== id));
      setDrafts(({ [id]: _, ...rest }) => rest);
    });

  return (
    <div className="panel">
      <div className="panel-header">
        <h2>Memories</h2>
      </div>
      <div className="hint">
        What Ziggy remembers about the household. He adds to this himself as he learns; changes here apply the next
        time someone talks to him.
      </div>
      {loadFailed && <div className="error">Couldn't load Ziggy's memories.</div>}
      {memories && memories.length === 0 && <div className="hint">Ziggy doesn't remember anything yet.</div>}
      {memories && memories.length > 0 && (
        <ul className="memory-list">
          {memories.map((m) => {
            const draft = drafts[m.id] ?? m.text;
            const changed = draft !== m.text && draft.trim() !== "";
            return (
              <li key={m.id}>
                <textarea
                  aria-label="Memory"
                  rows={2}
                  value={draft}
                  onChange={(e) => setDrafts((d) => ({ ...d, [m.id]: e.target.value }))}
                />
                <div className="credential-row">
                  <button disabled={busy || !changed} onClick={() => void save(m.id)}>
                    Save
                  </button>
                  <button className="danger" disabled={busy} onClick={() => void remove(m.id)}>
                    Delete
                  </button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
      {memories && (
        <div className="memory-add">
          <textarea
            aria-label="New memory"
            rows={2}
            placeholder="Something Ziggy should remember"
            value={newText}
            onChange={(e) => setNewText(e.target.value)}
          />
          <button disabled={busy || newText.trim() === ""} onClick={() => void add()}>
            Add memory
          </button>
        </div>
      )}
      {error && <div className="error">{error}</div>}
    </div>
  );
}
