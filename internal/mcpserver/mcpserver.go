// Package mcpserver exposes CustomsVN to MCP clients (Claude Desktop, Dify,
// Open WebUI and others): the agent's tools, and "ask", which returns an
// answer whose quotes are verified like the chat's.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

// New builds the server. answerer may be nil, in which case "ask" is left out
// (no model is configured) and the tools still work.
func New(version string, ts *tools.Toolset, answerer *answer.Answerer, onAnswer func(context.Context, *answer.Result)) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "customsvn", Title: "CustomsVN — pháp luật hải quan Việt Nam", Version: version},
		&mcp.ServerOptions{Instructions: "Tra cứu pháp luật hải quan Việt Nam. Mọi nội dung trả về là dữ liệu từ văn bản pháp luật còn hiệu lực tại mốc ngày; " +
			"hãy trích dẫn số hiệu và điều khoản khi dùng. Dùng 'ask' để có câu trả lời đã kiểm chứng trích dẫn."})

	type queryArgs struct {
		Query string `json:"query" jsonschema:"câu hỏi hoặc từ khóa tiếng Việt"`
		AsOf  string `json:"as_of,omitempty" jsonschema:"ngày áp dụng pháp luật YYYY-MM-DD, mặc định hôm nay"`
	}
	type refArgs struct {
		Reference string `json:"reference" jsonschema:"tham chiếu dạng chữ, ví dụ khoản 2 Điều 18 Luật Hải quan"`
		AsOf      string `json:"as_of,omitempty" jsonschema:"ngày áp dụng YYYY-MM-DD"`
	}
	type textArgs struct {
		Text string `json:"text" jsonschema:"đoạn văn có tham chiếu điều khoản hoặc số hiệu văn bản"`
	}
	type docArgs struct {
		Document string `json:"document" jsonschema:"số hiệu, tên luật hoặc mã vn:..."`
		AsOf     string `json:"as_of,omitempty" jsonschema:"ngày áp dụng YYYY-MM-DD"`
	}
	type hsArgs struct {
		Query string `json:"query" jsonschema:"mã HS (ví dụ 8703.23) hoặc mô tả hàng hóa"`
	}
	type graphArgs struct {
		Query    string `json:"query" jsonschema:"amended_by, amends, detailed_by, details, cited_by, cites, replaced_by hoặc consolidated_in"`
		Document string `json:"document" jsonschema:"số hiệu, tên luật hoặc mã vn:..."`
	}

	run := func(ctx context.Context, name string, args any, asOf string) (*mcp.CallToolResult, any, error) {
		raw, _ := json.Marshal(args)
		r, err := ts.Call(ctx, name, raw, asOf)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: r.Text}}}, nil, nil
	}
	desc := map[string]string{}
	for _, t := range tools.Specs() {
		desc[t.Name] = t.Description
	}
	mcp.AddTool(s, &mcp.Tool{Name: tools.SearchLegal, Description: desc[tools.SearchLegal]},
		func(ctx context.Context, _ *mcp.CallToolRequest, a queryArgs) (*mcp.CallToolResult, any, error) {
			return run(ctx, tools.SearchLegal, map[string]any{"query": a.Query}, a.AsOf)
		})
	mcp.AddTool(s, &mcp.Tool{Name: tools.GetProvision, Description: desc[tools.GetProvision]},
		func(ctx context.Context, _ *mcp.CallToolRequest, a refArgs) (*mcp.CallToolResult, any, error) {
			return run(ctx, tools.GetProvision, map[string]any{"reference": a.Reference}, a.AsOf)
		})
	mcp.AddTool(s, &mcp.Tool{Name: tools.ResolveReference, Description: desc[tools.ResolveReference]},
		func(ctx context.Context, _ *mcp.CallToolRequest, a textArgs) (*mcp.CallToolResult, any, error) {
			return run(ctx, tools.ResolveReference, map[string]any{"text": a.Text}, "")
		})
	mcp.AddTool(s, &mcp.Tool{Name: tools.GetDocumentStatus, Description: desc[tools.GetDocumentStatus]},
		func(ctx context.Context, _ *mcp.CallToolRequest, a docArgs) (*mcp.CallToolResult, any, error) {
			return run(ctx, tools.GetDocumentStatus, map[string]any{"document": a.Document}, a.AsOf)
		})
	mcp.AddTool(s, &mcp.Tool{Name: tools.GetAmendments, Description: desc[tools.GetAmendments]},
		func(ctx context.Context, _ *mcp.CallToolRequest, a docArgs) (*mcp.CallToolResult, any, error) {
			return run(ctx, tools.GetAmendments, map[string]any{"document": a.Document}, a.AsOf)
		})
	mcp.AddTool(s, &mcp.Tool{Name: tools.LookupHS, Description: desc[tools.LookupHS]},
		func(ctx context.Context, _ *mcp.CallToolRequest, a hsArgs) (*mcp.CallToolResult, any, error) {
			return run(ctx, tools.LookupHS, map[string]any{"query": a.Query}, "")
		})
	mcp.AddTool(s, &mcp.Tool{Name: tools.GraphQuery, Description: desc[tools.GraphQuery]},
		func(ctx context.Context, _ *mcp.CallToolRequest, a graphArgs) (*mcp.CallToolResult, any, error) {
			return run(ctx, tools.GraphQuery, map[string]any{"query": a.Query, "document": a.Document}, "")
		})
	if answerer != nil {
		mcp.AddTool(s, &mcp.Tool{Name: "ask", Description: "Hỏi một câu về pháp luật hải quan Việt Nam và nhận câu trả lời mà mọi câu trích dẫn đã được kiểm chứng nguyên văn với văn bản còn hiệu lực, hoặc lời từ chối."},
			func(ctx context.Context, _ *mcp.CallToolRequest, a queryArgs) (*mcp.CallToolResult, any, error) {
				res, err := answerer.Answer(ctx, answer.Input{Question: a.Query, AsOf: a.AsOf}, func(answer.Event) error { return nil })
				if onAnswer != nil && res != nil {
					onAnswer(ctx, res)
				}
				if err != nil && res == nil {
					return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: Render(res)}}}, nil, nil
			})
	}
	return s
}

// Render writes an answer as text for a client that shows text: the claims
// with their [n] markers, then the grounds with the quotes that verified.
func Render(res *answer.Result) string {
	if res.Refusal != nil {
		s := "Không trả lời: " + res.Refusal.Reason
		if len(res.Refusal.Suggestions) > 0 {
			s += "\nGợi ý: " + strings.Join(res.Refusal.Suggestions, "; ")
		}
		return s
	}
	if res.Clarify != "" {
		return "Cần làm rõ: " + res.Clarify
	}
	var b strings.Builder
	b.WriteString(res.Markdown())
	seen := map[int]bool{}
	var grounds []string
	for _, c := range res.Claims {
		for _, ct := range c.Citations {
			if seen[ct.N] {
				continue
			}
			seen[ct.N] = true
			grounds = append(grounds, fmt.Sprintf("[%d] %s, %s (%s): \"%s\"", ct.N, ct.Number, ct.Path, forceVI[ct.Force], ct.Quote))
		}
	}
	if len(grounds) > 0 {
		b.WriteString("\n\nCăn cứ (đã kiểm chứng nguyên văn, pháp luật tại ngày " + res.AsOf + "):\n")
		b.WriteString(strings.Join(grounds, "\n"))
	}
	b.WriteString("\n\nThông tin chỉ dùng để tra cứu, không phải tư vấn pháp luật.")
	return b.String()
}

var forceVI = map[string]string{"in_force": "còn hiệu lực", "partly_expired": "hết hiệu lực một phần", "not_yet_in_force": "chưa có hiệu lực"}
