package main

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

// Event represents a logged action in the database.
type Event struct {
	ID        int    `json:"id"`
	AgentID   int    `json:"agent_id"`
	Command   string `json:"command"`
	Payload   string `json:"payload"`
	Response  string `json:"response"`
	CreatedAt string `json:"created_at"`
}

// User represents an authenticated API user.
type User struct {
	ID       int
	Username string
	Password string
}

// ConnectDB opens a connection pool to PostgreSQL and verifies it.
func ConnectDB(dsn string) error {
	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	return db.Ping()
}

// LogEvent inserts an event record. Called as a goroutine (non-blocking).
func LogEvent(agentID int, command, payload, response string) {
	if db == nil {
		return
	}
	_, err := db.Exec(
		"INSERT INTO events (agent_id, command, payload, response) VALUES ($1, $2, $3, $4)",
		agentID, command, payload, response,
	)
	if err != nil {
		log.Printf("Failed to log event: %v", err)
	}
}

// QueryEvents retrieves events, optionally filtered by agent_id.
func QueryEvents(agentID int, limit int) ([]Event, error) {
	var rows *sql.Rows
	var err error

	if agentID > 0 {
		rows, err = db.Query(
			"SELECT id, agent_id, command, payload, response, created_at FROM events WHERE agent_id = $1 ORDER BY created_at DESC LIMIT $2",
			agentID, limit,
		)
	} else {
		rows, err = db.Query(
			"SELECT id, agent_id, command, payload, response, created_at FROM events ORDER BY created_at DESC LIMIT $1",
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var t time.Time
		if err := rows.Scan(&e.ID, &e.AgentID, &e.Command, &e.Payload, &e.Response, &t); err != nil {
			return nil, err
		}
		e.CreatedAt = t.Format("2006-01-02 15:04:05")
		events = append(events, e)
	}
	return events, nil
}

// CloseDB closes the database connection pool.
func CloseDB() {
	if db != nil {
		db.Close()
	}
}

// CreateUser inserts a new user with a pre-hashed password.
func CreateUser(username, hashedPassword string) error {
	_, err := db.Exec(
		"INSERT INTO users (username, password) VALUES ($1, $2)",
		username, hashedPassword,
	)
	return err
}

// FindUserByUsername returns a user by username, or (nil, nil) when not found.
func FindUserByUsername(username string) (*User, error) {
	row := db.QueryRow(
		"SELECT id, username, password FROM users WHERE username = $1",
		username,
	)

	var user User
	if err := row.Scan(&user.ID, &user.Username, &user.Password); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}
