# Goban Migration Scripts

These scripts handle migrating data from SQLite to PostgreSQL (and back if needed).

## Prerequisites

```bash
pip install psycopg2-binary  # Required for PostgreSQL operations
```

## Workflow Overview

1. **Export** - Dump existing SQLite data to JSON backup file
2. **Import** - Load JSON into PostgreSQL with type conversions
3. **Verify** - Compare record counts between source and target
4. **(Optional) Rollback** - Restore from PostgreSQL back to SQLite if needed

## Step-by-Step Migration Guide

### 1. Export from SQLite (Backup First!)

```bash
python3 migrate_sqlite_to_postgres.py export \
    --source /path/to/goban.db \
    --output goban_backup_$(date +%Y%m%d).json
```

This creates a JSON file with all tickets and tokens, preserving:
- All ticket fields (title, description, labels, subtasks, comments)
- Timestamps in ISO format
- Original IDs as references (new UUIDs generated for PostgreSQL)

### 2. Import to PostgreSQL (Dry Run First!)

```bash
# Preview what will be imported (no changes made)
python3 migrate_sqlite_to_postgres.py import \
    --input goban_backup_YYYYMMDD.json \
    --postgres "postgresql://kanban:password@localhost:5432/goban_test" \
    --dry-run

# Actually perform the import
python3 migrate_sqlite_to_postgres.py import \
    --input goban_backup_YYYYMMDD.json \
    --postgres "postgresql://kanban:password@localhost:5432/goban_test"
```

### 3. Verify Migration Integrity

```bash
python3 migrate_sqlite_to_postgres.py verify \
    --sqlite /path/to/goban.db \
    --postgres "postgresql://kanban:password@localhost:5432/goban_test"
```

This compares record counts, field types, and data integrity between source and target.

### 4. (Optional) Rollback to SQLite

If you need to revert back to SQLite after migrating to PostgreSQL:

```bash
python3 rollback_postgres_to_sqlite.py \
    --postgres "postgresql://kanban:password@localhost:5432/goban_test" \
    --sqlite /path/to/goban.db
```

This will:
- Auto-backup the current SQLite file with timestamp suffix
- Clear existing tables and import from PostgreSQL
- Convert types back (UUID to TEXT, JSONB to TEXT, TIMESTAMP to DATETIME)

## Type Conversions

| Field       | SQLite      | PostgreSQL  | Notes                              |
|-------------|-------------|-------------|------------------------------------|
| id          | TEXT        | UUID        | New UUID generated, old kept as ref|
| labels      | TEXT (JSON) | JSONB       | Array of strings                   |
| subtasks    | TEXT (JSON) | JSONB       | Nested array structure             |
| comments    | TEXT (JSON) | JSONB       | Nested array with timestamps       |
| due_date    | TEXT        | TIMESTAMP   | ISO format                         |
| created_at  | TEXT        | TIMESTAMP   | ISO format                         |
| updated_at  | TEXT        | TIMESTAMP   | ISO format                         |

## Troubleshooting

**"role does not exist" error**: PostgreSQL user must be created first
```bash
sudo -u postgres createuser -s kanban
sudo -u postgres psql -c "CREATE DATABASE goban_test OWNER kanban;"
```

**Connection refused**: Ensure PostgreSQL is running
```bash
systemctl status postgresql
```

**psycopg2 not installed**:
```bash
pip install psycopg2-binary
```

## Notes

- Migration scripts generate new UUIDs for tickets in PostgreSQL
- Original SQLite IDs are preserved as `old_id` field in export JSON
- Token hashes remain unchanged (same credentials work after migration)
- Always test on staging database before production migration
