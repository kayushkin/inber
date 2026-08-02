package server

import (
	"fmt"
	"log"

	"github.com/kayushkin/inber/logger"
)

// releaseSession takes a session out of the live map and closes it.
//
// Removal and close belong together, and this exists because one site had them
// apart. `new_session` used to drop the session under the main key with a bare
// `g.sessions.Delete`, which is the only removal in this package that never
// paired with a close — the reaper, the lost race in getOrCreateSession, the
// bridge rebuild and shutdown all close what they remove. A dropped *Session
// just becomes unreachable, and Go's sql.DB has no finalizer, so the engine's
// memory store, session DB, forge DB and JSONL writers stay open for the life
// of the process. N `new_session` calls leaked N engines' worth of file
// descriptors on the same long-lived server the reaper exists to keep alive.
//
// The behavioural half is worse than the resource half. Engine.Close is what
// calls SaveSessionSummary, which distils a finished conversation into memory.
// Starting a fresh session is exactly the moment you would want the one you are
// replacing summarized, and it was the one path that never did.
//
// A session that is still running is removed but NOT closed: Session.close
// cancels the turn and then closes the engine's stores without waiting for the
// turn goroutine to notice, so closing under a live turn is a use-after-close.
// Callers get that case out of the way instead of handling it here — run
// releases inside the queue, whose per-session lock has already serialized it
// behind any turn on that key — so reaching this branch means a turn ran
// outside the queue, and the log says so rather than papering over it.
//
// Reports whether the session was closed.
func (g *Server) releaseSession(key string) bool {
	val, ok := g.sessions.LoadAndDelete(key)
	if !ok {
		return false
	}
	s := val.(*Session)

	s.mu.Lock()
	running := s.Status == Running
	s.mu.Unlock()

	if running {
		log.Printf("[server] released session %s while a turn was still running: "+
			"its engine is left open rather than closed under the turn, and it leaks", key)
		return false
	}

	s.close()
	return true
}

// injectIfBusy hands a message to a session that is already mid-turn, which is
// what the server does instead of queuing a second turn behind the first.
//
// A fresh-session request is refused this shortcut. It asked for a new
// conversation, and injecting into the running one would deliver the message to
// the very session the request wants replaced — so it goes to the queue, waits
// out the turn in flight, and the old session is released there.
//
// Reports whether the message was injected, in which case the response is the
// caller's answer.
func (g *Server) injectIfBusy(sessionKey, input string, req RunRequest) (*RunResponse, bool) {
	if req.NewSession {
		return nil, false
	}

	val, ok := g.sessions.Load(sessionKey)
	if !ok {
		return nil, false
	}
	sess := val.(*Session)

	// One act, not a test followed by a send. A session that reads Running and
	// is idle by the time the message is sent has no reader for it, and a full
	// buffer has no room for it; in both cases this returns false and the
	// message goes back down the ordinary path, where the queue serializes it
	// behind whatever turn is running and then runs it as a turn of its own.
	if !sess.injectIfRunning(input) {
		return nil, false
	}

	logger.WithComponent("server").Debug("session busy, message injected mid-turn", map[string]interface{}{
		"session_key": sessionKey,
	})
	return &RunResponse{
		Text:       "[Message injected into running session — agent will see it during current work]",
		SessionKey: sessionKey,
	}, true
}

// mainSessionKey is the key a session gets when the caller named none.
func mainSessionKey(agentName string) string {
	return fmt.Sprintf("agent:%s:main", agentName)
}
