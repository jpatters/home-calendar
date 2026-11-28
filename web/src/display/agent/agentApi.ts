export async function converse(query: string, conversationId: string): Promise<string> {
  const res = await fetch("/api/agent/converse", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ conversation_id: conversationId, query }),
  });
  if (!res.ok) throw new Error(`POST /api/agent/converse ${res.status}`);
  const body: { output: string } = await res.json();
  return body.output;
}
