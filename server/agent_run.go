package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/kayushkin/bus/messages"
)

// setupAgentRunHandler subscribes to "agent.run" NATS subject for scheduled agent turns.
func (g *Server) setupAgentRunHandler() {
	if g.bus == nil {
		return
	}

	err := g.bus.Reply("agent.run", func(data []byte) (any, error) {
		var req messages.AgentRunRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return messages.AgentRunResult{
				Success: false,
				Agent:   req.Agent,
				Error:   fmt.Sprintf("unmarshal: %v", err),
			}, nil
		}

		log.Printf("[agent.run] %s agent=%s target=%s job=%s", req.Prompt[:min(len(req.Prompt), 80)], req.Agent, req.SessionTarget, req.JobID)

		result := g.handleAgentRun(req)
		return result, nil
	})
	if err != nil {
		log.Printf("[agent.run] failed to subscribe: %v", err)
	} else {
		log.Printf("[agent.run] subscribed to agent.run")
	}
}

func (g *Server) handleAgentRun(req messages.AgentRunRequest) messages.AgentRunResult {
	// Resolve agent.
	ac, ok := g.GetAgentConfig(req.Agent)
	if !ok {
		return messages.AgentRunResult{
			Success: false,
			Agent:   req.Agent,
			Error:   fmt.Sprintf("unknown agent: %s", req.Agent),
		}
	}

	if req.Model != "" {
		ac.Model = req.Model
	}

	switch req.SessionTarget {
	case "main":
		return g.agentRunMain(req, ac)
	case "isolated":
		return g.agentRunIsolated(req, ac)
	default:
		return messages.AgentRunResult{
			Success: false,
			Agent:   req.Agent,
			Error:   fmt.Sprintf("unknown session_target: %q", req.SessionTarget),
		}
	}
}

// agentRunMain injects the prompt into the agent's main session and runs a turn.
func (g *Server) agentRunMain(req messages.AgentRunRequest, ac AgentConfig) messages.AgentRunResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	resp, err := g.Run(ctx, RunRequest{
		Agent:   req.Agent,
		Message: req.Prompt,
		Channel: "nats",
		Author:  "scheduler",
	})
	if err != nil {
		return messages.AgentRunResult{
			Success: false,
			Agent:   req.Agent,
			Error:   err.Error(),
		}
	}

	return messages.AgentRunResult{
		Success: true,
		Agent:   req.Agent,
		Output:  resp.Text,
	}
}

// agentRunIsolated spawns a new session, runs one turn, returns the result.
func (g *Server) agentRunIsolated(req messages.AgentRunRequest, ac AgentConfig) messages.AgentRunResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	sessionKey := fmt.Sprintf("agent:%s:isolated:%s:%d", req.Agent, req.JobID, time.Now().UnixMilli())

	// Ensure session exists in DB.
	g.store.UpsertSession(sessionKey, req.Agent, "isolated")

	sess, err := g.createSession(sessionKey, req.Agent, ac, nil)
	if err != nil {
		return messages.AgentRunResult{
			Success: false,
			Agent:   req.Agent,
			Error:   fmt.Sprintf("create session: %v", err),
		}
	}
	g.sessions.Store(sessionKey, sess)
	defer func() {
		g.sessions.Delete(sessionKey)
		sess.close()
	}()

	result, err := sess.turn(ctx, req.Prompt)
	if err != nil {
		return messages.AgentRunResult{
			Success: false,
			Agent:   req.Agent,
			Error:   err.Error(),
		}
	}

	return messages.AgentRunResult{
		Success: true,
		Agent:   req.Agent,
		Output:  result.Text,
	}
}
