package mcp

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"goban/config"
)

type listInput struct {
	BoardID   string `json:"board_id" jsonschema:"board to list from"`
	AuthToken string `json:"auth_token,omitempty" jsonschema:"optional JWT for protected operations"`
}

type listOutput struct {
	Tickets string `json:"tickets" jsonschema:"ticket list"`
}

func listTickets(ctx context.Context, req *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, listOutput, error) {
	return nil, listOutput{Tickets: "[]"}, nil
}

func Start(cfg config.Config) error {
	srv := mcp.NewServer(&mcp.Implementation{Name: "goban", Version: cfg.Version}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "list_tickets", Description: "List tickets on a board"}, listTickets)
	// Additional tools (create_ticket, claim_ticket, move_ticket, add_comment, list_boards) follow same pattern in later work
	if cfg.MCPTransport == "http" {
		log.Printf("[MCP] HTTP transport selected (implementation pending)")
		return nil
	}
	log.Printf("[MCP] Starting stdio server")
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
