#!/usr/bin/env python3
"""
Goban PostgreSQL to SQLite Rollback Script
Emergency rollback procedure - restores data from PostgreSQL back to SQLite

Usage:
    python3 rollback_postgres_to_sqlite.py --postgres "host=localhost..." --sqlite /path/to/goban.db
"""

import argparse
import json
import sqlite3
import sys

try:
    import psycopg2
    PSYCOPG2_AVAILABLE = True
except ImportError:
    PSYCOPG2_AVAILABLE = False


def rollback_postgres_to_sqlite(postgres_conn_str: str, sqlite_path: str):
    """Rollback: Import PostgreSQL data back to SQLite"""
    
    if not PSYCOPG2_AVAILABLE:
        print("Error: psycopg2 required. Install with: pip install psycopg2-binary")
        sys.exit(1)
    
    # Connect to PostgreSQL and export
    pg_conn = psycopg2.connect(postgres_conn_str)
    pg_cursor = pg_conn.cursor()
    
    # Get tickets from PostgreSQL
    pg_cursor.execute("""
        SELECT id, board_id, title, description, column_id, assignee, priority, 
               labels, due_date, subtasks, comments, created_at, updated_at, archived, archived_at
        FROM tickets
    """)
    
    tickets = []
    for row in pg_cursor.fetchall():
        # Convert JSONB to Python objects then back to SQLite TEXT format
        ticket = {
            "id": str(row[0]),  # UUID -> TEXT
            "board_id": row[1],
            "title": row[2],
            "description": row[3],
            "column_id": row[4],
            "assignee": row[5] or "",
            "priority": row[6] or "Medium",
            "labels": json.dumps(row[7]) if row[7] else "[]",  # JSONB -> TEXT
            "due_date": row[8].isoformat() if row[8] else None,  # TIMESTAMP -> TEXT
            "subtasks": json.dumps(row[9]) if row[9] else "[]",  # JSONB -> TEXT
            "comments": json.dumps(row[10]) if row[10] else "[]",  # JSONB -> TEXT
            "created_at": row[11].isoformat() if row[11] else None,
            "updated_at": row[12].isoformat() if row[12] else None,
            "archived": bool(row[13]),
            "archived_at": row[14].isoformat() if row[14] else None
        }
        tickets.append(ticket)
    
    # Get tokens from PostgreSQL
    pg_cursor.execute("""
        SELECT agent_name, token_hash, created_at, last_used FROM tokens
    """)
    
    tokens = []
    for row in pg_cursor.fetchall():
        tokens.append({
            "agent_name": row[0],
            "token_hash": row[1],
            "created_at": row[2].isoformat() if row[2] else None,
            "last_used": row[3].isoformat() if row[3] else None
        })
    
    pg_cursor.close()
    pg_conn.close()
    
    # Backup existing SQLite first
    import shutil
    backup_path = sqlite_path + ".backup." + str(int(datetime.now().timestamp()))
    try:
        shutil.copy2(sqlite_path, backup_path)
        print(f"Backed up existing SQLite to: {backup_path}")
    except FileNotFoundError:
        print("No existing SQLite file found (this is OK for fresh rollback)")
    
    # Create new SQLite database with PostgreSQL data
    sqlite_conn = sqlite3.connect(sqlite_path)
    sqlite_cursor = sqlite_conn.cursor()
    
    # Clear existing tables
    sqlite_cursor.execute("DELETE FROM tickets")
    sqlite_cursor.execute("DELETE FROM tokens")
    
    # Insert tickets
    for ticket in tickets:
        sqlite_cursor.execute("""
            INSERT INTO tickets (id, board_id, title, description, column_id, assignee, priority,
                               labels, due_date, subtasks, comments, archived, archived_at, created_at, updated_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, (
            ticket["id"], ticket["board_id"], ticket["title"], ticket["description"],
            ticket["column_id"], ticket["assignee"], ticket["priority"],
            ticket["labels"], ticket["due_date"], ticket["subtasks"], ticket["comments"],
            ticket["archived"], ticket["archived_at"], ticket["created_at"], ticket["updated_at"]
        ))
    
    # Insert tokens
    for token in tokens:
        sqlite_cursor.execute("""
            INSERT INTO tokens (agent_name, token_hash, created_at, last_used)
            VALUES (?, ?, ?, ?)
        """, (token["agent_name"], token["token_hash"], token["created_at"], token["last_used"]))
    
    sqlite_conn.commit()
    sqlite_conn.close()
    
    print(f"Rollback complete: {len(tickets)} tickets and {len(tokens)} tokens restored to SQLite")


def main():
    parser = argparse.ArgumentParser(description="Goban PostgreSQL to SQLite Rollback")
    parser.add_argument('--postgres', '-p', required=True, help='PostgreSQL connection string')
    parser.add_argument('--sqlite', '-s', required=True, help='Target SQLite database path')
    
    args = parser.parse_args()
    rollback_postgres_to_sqlite(args.postgres, args.sqlite)


if __name__ == '__main__':
    from datetime import datetime
    main()
