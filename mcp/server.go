package mcp

import (
	"context"
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"goban/config"
	"goban/store"
	"goban/version"
)

func Start(cfg config.Config, db store.TicketStore) error {
	SetAllowedBoards(cfg.Boards)
	ver := cfg.Version
	if ver == "" {
		ver = version.Version
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "goban", Version: ver}, nil)

	mcp.AddTool(srv, &mcp.Tool{Name: "list_tickets", Description: "List tickets, optionally filtered by board_id"},
		func(ctx context.Context, req *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, listOutput, error) {
			encoded, err := encodeTicketList(db, in.BoardID)
			if err != nil {
				return nil, listOutput{}, err
			}
			return nil, listOutput{Tickets: encoded}, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "list_boards", Description: "List configured boards"},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, listOutput, error) {
			encoded, err := listBoards(cfg, db)
			if err != nil {
				return nil, listOutput{}, err
			}
			return nil, listOutput{Tickets: encoded}, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "create_ticket", Description: "Create a ticket (requires auth_token)"},
		func(ctx context.Context, req *mcp.CallToolRequest, in createInput) (*mcp.CallToolResult, ticketResult, error) {
			t, err := createTicket(db, in)
			if err != nil {
				return nil, ticketResult{}, err
			}
			return nil, ticketResult{ID: t.ID, Title: t.Title, BoardID: t.BoardID, Column: t.Column}, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "claim_ticket", Description: "Claim a ticket (requires auth_token)"},
		func(ctx context.Context, req *mcp.CallToolRequest, in claimInput) (*mcp.CallToolResult, claimInput, error) {
			res, err := claimTicket(db, in)
			if err != nil {
				return nil, claimInput{}, err
			}
			id := ""
			if res != nil && res.Ticket != nil {
				id = res.Ticket.ID
			}
			return nil, claimInput{TicketID: id}, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "move_ticket", Description: "Move a ticket to a status (requires auth_token)"},
		func(ctx context.Context, req *mcp.CallToolRequest, in moveInput) (*mcp.CallToolResult, moveInput, error) {
			res, err := moveTicket(db, in)
			if err != nil {
				return nil, moveInput{}, err
			}
			id, col := "", ""
			if res != nil && res.Ticket != nil {
				id = res.Ticket.ID
				col = res.Ticket.Column
			}
			return nil, moveInput{TicketID: id, TargetStatus: col}, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "add_comment", Description: "Add a comment to a ticket (requires auth_token)"},
		func(ctx context.Context, req *mcp.CallToolRequest, in commentInput) (*mcp.CallToolResult, commentInput, error) {
			c, err := addComment(db, in)
			if err != nil {
				return nil, commentInput{}, err
			}
			return nil, commentInput{TicketID: in.TicketID, Text: c.Text}, nil
		})

	if cfg.MCPTransport == "http" {
		return fmt.Errorf("MCP HTTP transport is not implemented")
	}
	log.Printf("[MCP] Starting stdio server")
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
