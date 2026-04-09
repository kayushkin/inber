package server

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// forkSession creates a child session with a deep copy of the parent's messages.
func (g *Server) forkSession(parent *Session, childKey, agentName string, ac AgentConfig, onEvent func(StreamEvent)) (*Session, error) {
	// Deep copy parent's messages.
	parent.mu.Lock()
	msgData, err := json.Marshal(parent.Engine.Messages)
	parent.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("copy messages: %w", err)
	}

	var parentMessages []anthropic.MessageParam
	if err := json.Unmarshal(msgData, &parentMessages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}

	child, err := g.createSession(childKey, agentName, ac, RunRequest{}, onEvent)
	if err != nil {
		return nil, err
	}

	// Replace the empty messages with parent's history.
	child.Engine.Messages = parentMessages
	child.SpawnDepth = parent.SpawnDepth + 1
	child.ParentKey = parent.Key

	// Inject sub-agent context.
	taskContext := fmt.Sprintf("[System] You are a forked sub-agent. "+
		"You inherited your parent's conversation context. "+
		"Complete your assigned task and respond with your results. "+
		"Do not repeat context you already have.")
	child.Engine.Messages = append(child.Engine.Messages,
		anthropic.NewUserMessage(anthropic.NewTextBlock(taskContext)))

	return child, nil
}

// sessionKeyForChild generates a child session key.
func sessionKeyForChild(parentKey string) string {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	return parentKey + ":sub:" + suffix
}