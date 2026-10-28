// A fake OpenAI-compatible /v1/chat/completions for the end-to-end tests, so
// they need no API key and no network:
//
// - router calls (the system prompt classifies questions) get an intent;
// - agent calls (with tools) search once, then stop;
// - compose calls quote the first words of the first two sources, so the
//   quotes verify, and add one invented claim for the verifier to drop.
//
// Usage: node e2e/mock-llm.mjs [port]   (default 18090)
import http from "node:http";

const port = Number(process.argv[2] ?? 18090);
const SOURCE = /<source n="(\d+)">\n([\s\S]*?)\n---\n([\s\S]*?)\n<\/source>/g;

const question = (text) => (/<question>\n([\s\S]*?)\n<\/question>/.exec(text)?.[1] ?? text).trim();

function route(q) {
  const l = q.toLowerCase();
  if (l.includes("giá vàng")) return { intent: "out_of_scope", clarify: "" };
  if (l.includes("thủ tục nhập khẩu nguyên liệu"))
    return {
      intent: "procedure",
      clarify: "Bạn nhập nguyên liệu theo loại hình kinh doanh, gia công hay sản xuất xuất khẩu?",
    };
  return { intent: "regulation", clarify: "" };
}

function reply(body) {
  const msgs = body.messages;
  const system = msgs[0]?.role === "system" ? msgs[0].content : "";
  const last = msgs[msgs.length - 1];
  const user = last.role === "user" ? last.content : "";
  if (system.includes("phân loại câu hỏi")) return { text: JSON.stringify(route(question(user))) };
  if (body.tools?.length) {
    if (msgs.some((m) => m.role === "tool")) return { text: "XONG" };
    const q = question(msgs.find((m) => m.role === "user").content);
    return {
      text: "",
      calls: [{ id: "call_1", name: "search_legal", arguments: JSON.stringify({ query: q + " quy định", k: 4 }) }],
    };
  }
  if (system.includes("viết lại"))
    return { text: "Người khai hải quan: " + question(user.split("CÂU HỎI CUỐI:").pop()) };
  if (user.includes("Trả lời đúng một từ")) return { text: "OK" };
  const lines = [];
  for (const [, n, head, text] of [...user.matchAll(SOURCE)].slice(0, 2)) {
    const quote = text.replace(/\n/g, " ").split(/\s+/).filter(Boolean).slice(0, 18).join(" ");
    const number = head
      .split("\n")[0]
      .split(" — ")[0]
      .replace(/^\[\d+\] /, "");
    lines.push(
      JSON.stringify({
        claim: `Theo **${number}**, nội dung liên quan như sau.`,
        citations: [{ source: Number(n), quote }],
      }),
    );
  }
  lines.push(
    JSON.stringify({
      claim: "Một nhận định bịa đặt.",
      citations: [{ source: 1, quote: "câu này không hề có trong văn bản" }],
    }),
  );
  lines.push('{"end": true}');
  return { text: lines.join("\n") };
}

http
  .createServer((req, res) => {
    let raw = "";
    req.on("data", (c) => (raw += c));
    req.on("end", async () => {
      const body = JSON.parse(raw || "{}");
      // Model listing, embeddings and rerank, for the model settings screen.
      const json = (o) => {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify(o));
      };
      if (req.url?.endsWith("/models"))
        return json({ data: ["mock-1", "mock-embed", "mock-rerank"].map((id) => ({ id })) });
      if (req.url?.endsWith("/embeddings"))
        return json({
          data: [].concat(body.input).map((t, index) => ({
            index,
            embedding: Array.from({ length: 8 }, (_, i) => ((t.length + i) % 7) / 7),
          })),
        });
      if (req.url?.endsWith("/rerank"))
        return json({
          results: body.documents.map((_, index) => ({ index, relevance_score: index === 0 ? 0.9 : 0.1 })),
        });
      const { text, calls } = reply(body);
      res.writeHead(200, { "Content-Type": "text/event-stream" });
      const send = (o) => res.write("data: " + JSON.stringify(o) + "\n\n");
      for (let i = 0; i < text.length; i += 24) {
        send({ model: body.model ?? "mock", choices: [{ delta: { content: text.slice(i, i + 24) } }] });
        await new Promise((r) => setTimeout(r, 5));
      }
      if (calls) {
        calls.forEach((c, i) =>
          send({
            choices: [
              { delta: { tool_calls: [{ index: i, id: c.id, function: { name: c.name, arguments: c.arguments } }] } },
            ],
          }),
        );
        send({ choices: [{ delta: {}, finish_reason: "tool_calls" }] });
      }
      res.end('data: {"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50}}\n\ndata: [DONE]\n\n');
    });
  })
  .listen(port, "0.0.0.0", () => console.log(`mock LLM on :${port}`));
