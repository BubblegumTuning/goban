#!/usr/bin/env python3
"""
Goban SQLite to PostgreSQL Migration Script
Phase 3 of PostgreSQL migration plan

Exports data from SQLite and imports to PostgreSQL with proper type conversions:
- TEXT id -> UUID (generates new UUID, keeps old as reference)
- TEXT labels/subtasks/comments -> JSONB
- DATETIME -> TIMESTAMP

Usage:
    python3 migrate_sqlite_to_postgres.py --export /path/to/goban.db --output backup.json
    python3 migrate_sqlite_to_postgres.py --import backup.json --postgres "host=localhost user=..." 

"""

import argparse
import json
import sqlite3
import sys
from datetime import datetime
import uuid

try:
    import psycopg2
    PSYCOPG2_AVAILABLE = True
except ImportError:
    PSYCOPG2_AVAILABLE = False
    print("Warning: psycopg2 not installed. Install with: pip install psycopg2-binary")


def export_sqlite_to_json(db_path: str, output_path: str) -> dict:
    """Export all tickets and tokens from SQLite to JSON"""
    print(f"Exporting from SQLite: {db_path}")
    
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()
    
    # Export tickets
    cursor.execute("""
        SELECT id, board_id, title, description, column_id, assignee, priority, 
               labels, due_date, subtasks, comments, created_at, updated_at, archived, archived_at
        FROM tickets
    """)
    
    tickets = []
    for row in cursor.fetchall():
        ticket = {
            "old_id": row[0],  # Keep original ID as reference
            "board_id": row[1],
            "title": row[2],
            "description": row[3],
            "column_id": row[4],
            "assignee": row[5] or "",
            "priority": row[6] or "Medium",
            "labels": json.loads(row[7]) if row[7] else [],
            "due_date": row[8],
            "subtasks": json.loads(row[9]) if row[9] else [],
            "comments": json.loads(row[10]) if row[10] else [],
            "created_at": row[11],
            "updated_at": row[12],
            "archived": bool(row[13]),
            "archived_at": row[14]
        }
        tickets.append(ticket)
    
    # Export tokens
    cursor.execute("""
        SELECT id, agent_name, token_hash, created_at, last_used
        FROM tokens
    """)
    
    tokens = []
    for row in cursor.fetchall():
        token = {
            "agent_name": row[1],
            "token_hash": row[2],
            "created_at": row[3],
            "last_used": row[4]
        }
        tokens.append(token)
    
    conn.close()
    
    export_data = {
        "export_timestamp": datetime.now().isoformat(),
        "source": "sqlite",
        "tickets": tickets,
        "tokens": tokens
    }
    
    with open(output_path, 'w') as f:
        json.dump(export_data, f, indent=2)
    
    print(f"Exported {len(tickets)} tickets and {len(tokens)} tokens to {output_path}")
    return export_data


def import_json_to_postgres(json_path: str, postgres_conn_str: str, dry_run: bool = False):
    """Import JSON data into PostgreSQL with type conversions"""
    
    if not PSYCOPG2_AVAILABLE:
        print("Error: psycopg2 required for PostgreSQL import. Install with: pip install psycopg2-binary")
        sys.exit(1)
    
    print(f"Importing from: {json_path}")
    
    with open(json_path, 'r') as f:
        data = json.load(f)
    
    # Connect to PostgreSQL
    conn = psycopg2.connect(postgres_conn_str)
    cursor = conn.cursor()
    
    try:
        tickets_imported = 0
        
        for ticket in data["tickets"]:
            # Generate new UUID for the ticket
            new_id = str(uuid.uuid4())
            
            # Convert JSON arrays to PostgreSQL JSONB format
            labels_jsonb = json.dumps(ticket["labels"])
            subtasks_jsonb = json.dumps(ticket["subtasks"])
            comments_jsonb = json.dumps(ticket["comments"])
            
            if dry_run:
                print(f"Would insert ticket: {ticket['title']} (old_id: {ticket['old_id']}, new_uuid: {new_id})")
                continue
            
            cursor.execute("""
                INSERT INTO tickets (id, board_id, title, description, column_id, assignee, priority, 
                                   labels, due_date, subtasks, comments, archived, archived_at, created_at, updated_at)
                VALUES (%s, %s, %s, %s, %s, %s, %s, %s::jsonb, %s, %s::jsonb, %s::jsonb, %s, %s, %s, %s)
            """, (
                new_id, ticket["board_id"], ticket["title"], ticket["description"], 
                ticket["column_id"], ticket["assignee"], ticket["priority"],
                labels_jsonb, ticket["due_date"], subtasks_jsonb, comments_jsonb,
                ticket["archived"], ticket["archived_at"], ticket["created_at"], ticket["updated_at"]
            ))
            
            tickets_imported += 1
        
        # Import tokens (hashes remain the same)
        tokens_imported = 0
        for token in data["tokens"]:
            if dry_run:
                print(f"Would insert token for agent: {token['agent_name']}")
                continue
            
            cursor.execute("""
                INSERT INTO tokens (agent_name, token_hash, created_at, last_used)
                VALUES (%s, %s, %s, %s)
                ON CONFLICT (agent_name) DO NOTHING
            """, (
                token["agent_name"], token["token_hash"], 
                token["created_at"], token["last_used"]
            ))
            
            tokens_imported += 1
        
        conn.commit()
        
        if not dry_run:
            print(f"Successfully imported {tickets_imported} tickets and {tokens_imported} tokens to PostgreSQL")
        
    except Exception as e:
        conn.rollback()
        print(f"Error during import: {e}")
        raise
    finally:
        cursor.close()
        conn.close()


def verify_migration(sqlite_path: str, postgres_conn_str: str):
    """Verify data integrity after migration"""
    
    if not PSYCOPG2_AVAILABLE:
        print("Error: psycopg2 required for verification")
        sys.exit(1)
    
    # Count SQLite records
    sqlite_conn = sqlite3.connect(sqlite_path)
    sqlite_cursor = sqlite_conn.cursor()
    sqlite_cursor.execute("SELECT COUNT(*) FROM tickets")
    sqlite_tickets = sqlite_cursor.fetchone()[0]
    sqlite_cursor.execute("SELECT COUNT(*) FROM tokens")
    sqlite_tokens = sqlite_cursor.fetchone()[0]
    sqlite_conn.close()
    
    # Count PostgreSQL records
    pg_conn = psycopg2.connect(postgres_conn_str)
    pg_cursor = pg_conn.cursor()
    pg_cursor.execute("SELECT COUNT(*) FROM tickets")
    pg_tickets = pg_cursor.fetchone()[0]
    pg_cursor.execute("SELECT COUNT(*) FROM tokens")
    pg_tokens = pg_cursor.fetchone()[0]
    pg_conn.close()
    
    print("\n=== Migration Verification ===")
    print(f"SQLite: {sqlite_tickets} tickets, {sqlite_tokens} tokens")
    print(f"PostgreSQL: {pg_tickets} tickets, {pg_tokens} tokens")
    
    if sqlite_tickets == pg_tickets and sqlite_tokens == pg_tokens:
        print("✅ Record counts match - migration successful!")
        return True
    else:
        print("❌ Record count mismatch - verify manually")
        return False


def main():
    parser = argparse.ArgumentParser(description="Goban SQLite to PostgreSQL Migration")
    
    subparsers = parser.add_subparsers(dest='command', help='Commands')
    
    # Export command
    export_parser = subparsers.add_parser('export', help='Export SQLite data to JSON')
    export_parser.add_argument('--source', '-s', required=True, help='Path to SQLite database')
    export_parser.add_argument('--output', '-o', required=True, help='Output JSON file path')
    
    # Import command
    import_parser = subparsers.add_parser('import', help='Import JSON data to PostgreSQL')
    import_parser.add_argument('--input', '-i', required=True, help='Input JSON file path')
    import_parser.add_argument('--postgres', '-p', required=True, help='PostgreSQL connection string')
    import_parser.add_argument('--dry-run', action='store_true', help='Show what would be imported without making changes')
    
    # Verify command
    verify_parser = subparsers.add_parser('verify', help='Verify migration integrity')
    verify_parser.add_argument('--sqlite', required=True, help='Original SQLite database path')
    verify_parser.add_argument('--postgres', '-p', required=True, help='PostgreSQL connection string')
    
    args = parser.parse_args()
    
    if args.command == 'export':
        export_sqlite_to_json(args.source, args.output)
        
    elif args.command == 'import':
        import_json_to_postgres(args.input, args.postgres, dry_run=args.dry_run)
        
    elif args.command == 'verify':
        verify_migration(args.sqlite, args.postgres)
    
    else:
        parser.print_help()


if __name__ == '__main__':
    main()
