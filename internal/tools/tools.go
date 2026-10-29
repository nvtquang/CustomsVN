// Package tools is what the agent, the MCP server and the API can ask the
// corpus: search it, fetch a provision, resolve a reference, read a
// document's status and amendment history, look up an HS code, and walk the
// relation graph. Every answer here comes from code and the database, never
// from a model, so a status or a timeline shown to a reader is a fact the
// data holds.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/embed"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/search"
)

// Toolset holds what the tools read.
type Toolset struct {
	Pool   *pgxpool.Pool
	Engine *search.Engine
	// HSEmbedder adds a vector ranking to HS description search. Nil means
	// the engine's default embedding model, if any.
	HSEmbedder embed.Embedder

	hsMu sync.Mutex
	hs   []HSLine // nil until loaded
}

// Reset drops what the toolset caches from the database, after an ingest.
func (t *Toolset) Reset() {
	t.hsMu.Lock()
	t.hs = nil
	t.hsMu.Unlock()
}

// Card is structured data the chat screen shows as it is: a status, a
// timeline, HS lines.
type Card struct {
	Type string `json:"type"` // status, amendments, hs, graph
	Data any    `json:"data"`
}

// Empty reports whether a card found nothing, such as an HS lookup with no
// line. An empty card tells the reader nothing and answers no question.
func (c Card) Empty() bool {
	if hs, ok := c.Data.(*HSResult); ok {
		return len(hs.Lines) == 0
	}
	return false
}

// Result is what one tool call produced.
type Result struct {
	// Text is what the model reads: compact, and marked as data.
	Text string
	// Summary is one line for the agent trace.
	Summary string
	Card    *Card
	// Sources are chunks the composer may cite.
	Sources []search.Hit
}

// ErrUnknownTool is a call to a tool that does not exist.
var ErrUnknownTool = errors.New("unknown tool")

// Names of the tools, as the model and MCP clients see them.
const (
	SearchLegal       = "search_legal"
	GetProvision      = "get_provision"
	ResolveReference  = "resolve_reference"
	GetDocumentStatus = "get_document_status"
	GetAmendments     = "get_amendments"
	LookupHS          = "lookup_hs"
	GraphQuery        = "graph_query"
)

func schema(props string, required ...string) json.RawMessage {
	req, _ := json.Marshal(required)
	return json.RawMessage(`{"type":"object","properties":{` + props + `},"required":` + string(req) + `}`)
}

// Specs describes the tools for a model that calls them.
func Specs() []llm.Tool {
	doc := `"document":{"type":"string","description":"Số hiệu (ví dụ 38/2015/TT-BTC), tên luật (ví dụ Luật Hải quan) hoặc mã định danh vn:..."}`
	return []llm.Tool{
		{Name: SearchLegal, Description: "Tìm các điều khoản còn hiệu lực liên quan đến một câu hỏi pháp luật hải quan. Trả về đoạn văn bản có thể trích dẫn.",
			Parameters: schema(`"query":{"type":"string","description":"câu hỏi hoặc từ khóa tiếng Việt"},"k":{"type":"integer","description":"số kết quả, tối đa 10"}`, "query")},
		{Name: GetProvision, Description: "Lấy nguyên văn một điều, khoản, điểm cụ thể, ví dụ \"khoản 2 Điều 18 Luật Hải quan\" hoặc \"Điều 25 Thông tư 38/2015/TT-BTC\". Ưu tiên văn bản hợp nhất.",
			Parameters: schema(`"reference":{"type":"string","description":"tham chiếu dạng chữ"}`, "reference")},
		{Name: ResolveReference, Description: "Chuyển tham chiếu dạng chữ thành văn bản và điều khoản trong cơ sở dữ liệu.",
			Parameters: schema(`"text":{"type":"string"}`, "text")},
		{Name: GetDocumentStatus, Description: "Trạng thái hiệu lực của một văn bản tại mốc ngày, văn bản hợp nhất và văn bản thay thế nó.",
			Parameters: schema(doc, "document")},
		{Name: GetAmendments, Description: "Chuỗi sửa đổi, bổ sung, thay thế, bãi bỏ, hợp nhất của một văn bản theo thời gian.",
			Parameters: schema(doc, "document")},
		{Name: LookupHS, Description: "Tra mã HS trong Danh mục hàng hóa xuất khẩu, nhập khẩu Việt Nam (TT 31/2022/TT-BTC): theo mã (ví dụ 8703.23) hoặc theo mô tả hàng hóa.",
			Parameters: schema(`"query":{"type":"string","description":"mã HS hoặc mô tả hàng hóa"}`, "query")},
		{Name: GraphQuery, Description: "Quan hệ giữa các văn bản: amended_by, amends, detailed_by, details, cited_by, cites, replaced_by, consolidated_in.",
			Parameters: schema(`"query":{"type":"string","enum":["amended_by","amends","detailed_by","details","cited_by","cites","replaced_by","consolidated_in"]},`+doc, "query", "document")},
	}
}

// Call runs one tool. asOf is the day force is judged on.
func (t *Toolset) Call(ctx context.Context, name string, args json.RawMessage, asOf string) (*Result, error) {
	if asOf == "" {
		asOf = t.Engine.Now().Format(time.DateOnly)
	}
	var a struct {
		Query     string `json:"query"`
		K         int    `json:"k"`
		Reference string `json:"reference"`
		Text      string `json:"text"`
		Document  string `json:"document"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, fmt.Errorf("%s: arguments: %w", name, err)
		}
	}
	switch name {
	case SearchLegal:
		return t.searchLegal(ctx, a.Query, a.K, asOf)
	case GetProvision:
		return t.getProvision(a.Reference, asOf)
	case ResolveReference:
		return t.resolveReference(a.Text, asOf)
	case GetDocumentStatus:
		return t.documentStatus(ctx, a.Document, asOf)
	case GetAmendments:
		return t.amendments(ctx, a.Document, asOf)
	case LookupHS:
		return t.lookupHS(ctx, a.Query)
	case GraphQuery:
		return t.graphQuery(ctx, a.Query, a.Document)
	}
	return nil, fmt.Errorf("%w %q", ErrUnknownTool, name)
}

func (t *Toolset) searchLegal(ctx context.Context, query string, k int, asOf string) (*Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("search_legal needs a query")
	}
	if k <= 0 || k > 10 {
		k = 8
	}
	res, err := t.Engine.Search(ctx, search.Request{Query: query, Filters: search.Filters{AsOf: asOf}, Config: search.Config{K: k}})
	if err != nil {
		return nil, err
	}
	var hits []search.Hit
	for _, h := range res.Hits {
		if h.Usable {
			hits = append(hits, h)
		}
	}
	return &Result{Text: hitsText(hits), Summary: fmt.Sprintf("%d điều khoản", len(hits)), Sources: hits}, nil
}

// hitsText lists chunks for the model, each with the identifier it can fetch
// again and enough text to judge relevance.
func hitsText(hits []search.Hit) string {
	if len(hits) == 0 {
		return "Không tìm thấy điều khoản còn hiệu lực."
	}
	var b strings.Builder
	for _, h := range hits {
		fmt.Fprintf(&b, "- %s | %s | %s\n  %s\n", h.Doc.OfficialNumber, strings.Join(h.Chunk.Path, " › "), forceVI[h.Force], truncate(oneLine(h.Chunk.Text), 600))
	}
	return b.String()
}

var forceVI = map[string]string{
	"in_force": "còn hiệu lực", "partly_expired": "hết hiệu lực một phần", "not_yet_in_force": "chưa có hiệu lực", "": "không còn hiệu lực",
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// JSON renders a value for the model.
func jsonText(v any) string {
	b, _ := json.MarshalIndent(v, "", " ")
	return string(b)
}
