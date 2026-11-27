import { afterEach, describe, expect, test, vi } from "vitest";
import { converse } from "./agentApi";

afterEach(() => vi.unstubAllGlobals());

function stubFetch(status: number, body: unknown) {
  const calls: { url: string; init: RequestInit | undefined }[] = [];
  vi.stubGlobal("fetch", async (url: string, init?: RequestInit) => {
    calls.push({ url, init });
    return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
  });
  return calls;
}

describe("converse", () => {
  test("posts the query and conversation id and returns the agent's output", async () => {
    const calls = stubFetch(200, { output: "It's sunny." });

    const out = await converse("what's the weather?", "conv-1");

    expect(out).toBe("It's sunny.");
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("/api/agent/converse");
    expect(calls[0].init?.method).toBe("POST");
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({
      conversation_id: "conv-1",
      query: "what's the weather?",
    });
  });

  test("rejects when the server responds with an error status", async () => {
    stubFetch(500, { error: "boom" });
    await expect(converse("hi", "conv-1")).rejects.toThrow(/500/);
  });
});
