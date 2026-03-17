package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/kayushkin/inber/engine"
	"github.com/spf13/cobra"
)

var btwAddr string

var btwCmd = &cobra.Command{
	Use:   "btw <session-key> <message>",
	Short: "Inject a message into a running session",
	Long: `Send a message to a running inber session via the server API.

Examples:
  inber btw my-session "hey, also check the tests"
  inber btw --addr :9000 my-session "one more thing"

If no arguments are given, lists active sessions.`,
	Args: cobra.ArbitraryArgs,
	Run:  runBtw,
}

func init() {
	btwCmd.Flags().StringVar(&btwAddr, "addr", "localhost:8200", "Server address")
}

func runBtw(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		listActiveSessions()
		fmt.Fprintf(os.Stderr, "\nUsage: inber btw <session-key> <message>\n")
		os.Exit(1)
	}

	sessionKey := args[0]

	if len(args) < 2 {
		listActiveSessions()
		fmt.Fprintf(os.Stderr, "\nUsage: inber btw <session-key> <message>\n")
		os.Exit(1)
	}

	message := strings.Join(args[1:], " ")

	addr := btwAddr
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}

	url := fmt.Sprintf("%s/api/sessions/%s/inject", addr, sessionKey)

	body, _ := json.Marshal(map[string]string{"message": message})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		engine.Log.Error("failed to connect: %v", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		engine.Log.Error("server returned %d: %s", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	fmt.Println("Injected.")
}

func listActiveSessions() {
	addr := btwAddr
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}

	resp, err := http.Get(addr + "/api/sessions")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not reach server at %s: %v\n", addr, err)
		return
	}
	defer resp.Body.Close()

	var sessions []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse sessions: %v\n", err)
		return
	}

	if len(sessions) == 0 {
		fmt.Println("No active sessions.")
		return
	}

	fmt.Printf("Active sessions (%d):\n", len(sessions))
	for _, s := range sessions {
		key, _ := s["key"].(string)
		agent, _ := s["agent"].(string)
		status, _ := s["status"].(string)
		fmt.Printf("  %s%-30s%s  agent=%-10s  status=%s\n", engine.Dim, key, engine.Reset, agent, status)
	}
}
