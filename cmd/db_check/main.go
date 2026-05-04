package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := sql.Open("sqlite3", "/home/nanami/goban/goban.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("=== Tables ===")
	rows, _ := db.Query("SELECT name FROM sqlite_master WHERE type='table';")
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Printf("Warning: failed to scan table name: %v", err)
			continue
		}
		fmt.Printf("  - %s\n", name)
	}
	rows.Close()

	fmt.Println("\n=== Users Schema ===")
	var schema string
	err = db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='users';").Scan(&schema)
	if err != nil || schema == "" {
		fmt.Println("Users table does not exist or query failed")
	} else {
		fmt.Println(schema)
	}

	fmt.Println("\n=== Users Data ===")
	userRows, _ := db.Query("SELECT id, name, role, password_hash FROM users;")
	for userRows.Next() {
		var id int64
		var name, role string
		var passHash sql.NullString
		if err := userRows.Scan(&id, &name, &role, &passHash); err != nil {
			log.Printf("Warning: failed to scan user row: %v", err)
			continue
		}
		fmt.Printf("  ID:%d Name:%s Role:%s PasswordHash:%v\n", id, name, role, passHash.Valid)
	}
	userRows.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		log.Printf("Warning: failed to count users: %v", err)
	}
	fmt.Printf("\nTotal users: %d\n", count)
}
