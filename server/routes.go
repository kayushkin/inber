package server

import (
	"database/sql"
	"log"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// RouteTable maps channels to agents. Used to resolve which agent handles
// an inbound message when the message doesn't specify an agent explicitly.
//
// Routes are stored in SQLite for persistence and loaded into memory for
// fast lookups. The in-memory copy is the hot path; DB is source of truth.
type RouteTable struct {
	db     *sql.DB
	routes map[string]string // channel → agent name
	mu     sync.RWMutex
}

// NewRouteTable opens or creates the routing database.
// Pass empty path to use in-memory only.
func NewRouteTable(dbPath string) (*RouteTable, error) {
	dsn := ":memory:"
	if dbPath != "" {
		dsn = dbPath + "?_journal_mode=WAL"
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS routes (
			channel TEXT PRIMARY KEY,
			agent TEXT NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	rt := &RouteTable{
		db:     db,
		routes: make(map[string]string),
	}

	// Load existing routes into memory.
	rows, err := db.Query("SELECT channel, agent FROM routes")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ch, agent string
			if rows.Scan(&ch, &agent) == nil {
				rt.routes[ch] = agent
			}
		}
	}

	if len(rt.routes) > 0 {
		log.Printf("[routes] loaded %d routes", len(rt.routes))
	}

	return rt, nil
}

// Resolve returns the agent name for a channel. Uses prefix matching:
// "discord:12345" matches a route for "discord". Returns empty string if
// no route matches.
func (rt *RouteTable) Resolve(channel string) string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Exact match first.
	if agent, ok := rt.routes[channel]; ok {
		return agent
	}

	// Prefix match: "discord:12345" → check "discord".
	if idx := strings.IndexByte(channel, ':'); idx > 0 {
		prefix := channel[:idx]
		if agent, ok := rt.routes[prefix]; ok {
			return agent
		}
	}

	return ""
}

// Set adds or updates a route.
func (rt *RouteTable) Set(channel, agent string) error {
	rt.mu.Lock()
	rt.routes[channel] = agent
	rt.mu.Unlock()

	_, err := rt.db.Exec(
		"INSERT INTO routes (channel, agent) VALUES (?, ?) ON CONFLICT(channel) DO UPDATE SET agent=excluded.agent",
		channel, agent)
	return err
}

// Delete removes a route.
func (rt *RouteTable) Delete(channel string) error {
	rt.mu.Lock()
	delete(rt.routes, channel)
	rt.mu.Unlock()

	_, err := rt.db.Exec("DELETE FROM routes WHERE channel = ?", channel)
	return err
}

// List returns all routes.
func (rt *RouteTable) List() map[string]string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	out := make(map[string]string, len(rt.routes))
	for ch, agent := range rt.routes {
		out[ch] = agent
	}
	return out
}

// Close closes the database.
func (rt *RouteTable) Close() error {
	return rt.db.Close()
}
