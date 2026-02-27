# The Míl Adventurers System

**Agents as characters, not tools.**

## Philosophy

In traditional AI agent systems, agents are job descriptions: "coder", "tester", "deployer". They're functional but lifeless.

In inber, agents are **Míl** (Milesians) — adventurers from Irish mythology. They have:
- **Names** — not roles
- **Personalities** — distinct voices and quirks
- **Character arcs** — they grow through quests
- **Visual identity** — pixel art avatars
- **Strengths and weaknesses** — no perfect agents

This isn't just flavor text. It fundamentally changes how you interact with them.

## Why Characters?

### 1. More Natural Delegation

Compare:
```bash
# Role-based (sterile)
inber -a coder "implement auth"

# Character-based (intuitive)
inber -a fionn "implement auth"
```

You're not invoking a function. You're asking **Fionn** for help. It feels different.

### 2. Distinct Communication Styles

Each character speaks differently:

**Fionn (Scholar):**
```
Implemented JWT authentication.
Files: auth.go, middleware.go
Tests pass. Ready for validation.
```

**Scáthach (Sentinel):**
```
❌ FAIL: 3 of 8 tests failed

TestUserAuth_ExpiredToken: FAILED
  Expected: 401 Unauthorized
  Got: 500 Internal Server Error
  Panic: runtime error: invalid memory address

Issue: No nil checks in validateToken()
Recommendation: Add input validation
```

**Oisín (Courier):**
```
🚀 SHIPPING IT!

✅ Committed: feat: JWT authentication (a7b3c4f)
✅ Deployed to: production
✅ Health check: 200 OK

🎉 LIVE! Users can now authenticate.
```

Same pipeline, three different voices. You know who's talking.

### 3. Trade-offs Make Them Real

Perfect agents are boring. Our adventurers have flaws:

- **Fionn:** Perfectionist, slow when speed matters
- **Scáthach:** Pessimistic, might block progress for 100% coverage
- **Oisín:** Impatient, might skip verification steps
- **Bran:** Can over-plan instead of executing

These aren't bugs—they're features. They make delegation decisions meaningful.

### 4. Character Progression

Agents aren't static. They track their quests:

```markdown
## Quest Log
- 2026-02-27: Implemented JWT auth (XP: +50)
- 2026-02-28: Fixed memory leak (XP: +30)
- 2026-03-01: Refactored middleware (XP: +40)

## Level: 2
## XP: 120
```

You can see what they've accomplished. They have **history**.

In the future, this could affect behavior:
- Level 5 Fionn might specialize in auth code
- Level 10 Scáthach might focus on performance testing
- Their avatars could evolve visually

## The Core Party

### Fionn the Scholar
**Inspired by:** Fionn mac Cumhaill (warrior-scholar who gained wisdom)  
**Class:** Scribe • Scholar • Code-Weaver  
**Role:** Implementation  
**Tools:** Full access (shell, files, memory)  
**Voice:** Terse, precise, humble

### Scáthach the Sentinel
**Inspired by:** Scáthach (legendary warrior-woman who trained heroes)  
**Class:** Guardian • Sentinel • Test-Warrior  
**Role:** Validation  
**Tools:** Read + shell (for running tests)  
**Voice:** Direct, detailed, protective

### Oisín the Courier
**Inspired by:** Oisín (poet who journeyed to Tír na nÓg)  
**Class:** Ranger • Courier • Ship-Master  
**Role:** Deployment  
**Tools:** Shell + git + read  
**Voice:** Energetic, confident, action-focused

### Bran the Strategist
**Inspired by:** Bran the Blessed (king and strategist)  
**Class:** Commander • Strategist • Quest-Master  
**Role:** Orchestration  
**Tools:** spawn_agent, read, memory  
**Voice:** Structured, calm, campaign-minded

## How Orchestration Works

When you give Bran a quest:
```bash
inber -a bran "add user authentication to the API"
```

He doesn't do it himself. He orchestrates:

```
📋 QUEST: Add user authentication

🎯 PHASE 1: Implementation
spawn_agent("fionn", "Implement JWT-based user authentication")
→ Fionn writes auth.go, middleware.go, tests

🎯 PHASE 2: Validation
spawn_agent("scathach", "Test the authentication implementation")
→ Scáthach runs tests
→ ❌ 2 failures: nil checks missing

🎯 PHASE 3: Fix & Retest
spawn_agent("fionn", "Fix nil check issues in validateToken()")
→ Fionn fixes bugs
spawn_agent("scathach", "Re-test authentication")
→ ✅ All tests pass

🎯 PHASE 4: Deployment
spawn_agent("oisin", "Deploy authentication feature to production")
→ Oisín commits, pushes, deploys
→ ✅ Live at api.example.com

✅ QUEST COMPLETE
```

This is the **coding manager pipeline** in action.

## The spawn_agent Tool

The magic that makes orchestration work:

```go
// In Bran's system prompt context
spawn_agent(agent: "fionn", task: "implement X", timeout: 300)
```

Under the hood:
1. Creates a fresh agent instance (Fionn)
2. Loads Fionn's context (identity + project context)
3. Sends the task as user message
4. Runs Fionn's agent loop (with his tools)
5. Returns result to Bran
6. Bran decides next step based on result

Each spawn is **isolated**. Fionn doesn't see Scáthach's context or Oisín's session.

## Character Sheets (Technical)

Each character has:

### Identity File (Markdown)
`agents/fionn.md` or `agents/templates/fionn.md`
```markdown
# Fionn the Scholar

**Class:** Scribe • Scholar • Code-Weaver

## Character
[Personality, background, beliefs]

## Communication Style
[How they speak, example outputs]

## Abilities
[What they're good at]

## Tools of the Trade
[Which tools they use]

## Weaknesses
[What they struggle with]

## Quest Log
- (logged adventures)

## Level: 1
## XP: 0
```

### Configuration (JSON)
`agents.json`
```json
"fionn": {
  "name": "fionn",
  "role": "scholar — code implementation specialist",
  "model": "claude-sonnet-4-5",
  "thinking": 0,
  "tools": ["shell", "read_file", "write_file", "edit_file", "list_files"],
  "context": {
    "tags": ["code", "errors", "debugging"],
    "budget": 50000
  }
}
```

### Avatar (Pixel Art)
`agents/avatars/fionn.png`
- 64x64 pixel art style
- Fantasy RPG aesthetic
- Matches character class/personality

## Creating New Characters

Follow the guide in `agents/templates/README.md`.

Key steps:
1. Pick an Irish/Celtic name
2. Define class and specialty
3. Write personality and quirks
4. Choose tools (match role)
5. Generate pixel art avatar
6. Test with sample task

Examples from mythology:
- **Brigid** — Triple goddess (poetry, healing, smithcraft) → multi-role specialist
- **Cú Chulainn** — Legendary warrior → defense/security specialist
- **Lugh** — God of skill and craft → debugger/fixer
- **Nuada** — King with silver arm → adaptability/resilience specialist

## Roadmap: Character Evolution

Future ideas:

### XP-Based Specialization
- Characters gain XP from completed quests
- Higher levels → specialized in domains they work in
- Level 10 Fionn who mostly writes auth code → becomes auth specialist
- Affects tool selection, context loading, even model choice

### Visual Progression
- Avatars evolve as characters level up
- New equipment, poses, effects
- Generated via image API with prompt: "Level 5 version of..."

### Memory-Driven Personality Drift
- Characters remember their quests
- Successful patterns reinforced
- Failures inform caution in similar situations
- "Fionn learned to add nil checks after the auth bug incident"

### Inter-Character Relationships
- Track which agents work together often
- Bran learns which specialists pair well
- "Fionn + Scáthach is a good combo for critical features"

### Narrative Logs
- Quest logs become story-like
- "Fionn ventured into the auth codebase, armed with knowledge of JWT..."
- Generated summaries with personality

## Technical Implementation

### Character Loading
```go
// Registry loads character identity from markdown
cfg, _ := registry.LoadConfig("agents.json", "agents/")

// Each agent's .md file becomes their system prompt
// Personality, voice, strengths, weaknesses all in the prompt
```

### Spawn Isolation
```go
// Each spawn gets fresh context
result, err := registry.SpawnAndRun(ctx, "fionn", "implement auth")

// Fionn's session is separate from Bran's
// No context leakage between agents
```

### Quest Logging (Manual for Now)
```markdown
## Quest Log
- 2026-02-27: Implemented JWT auth (XP: +50)
```

Future: Automatic logging on successful completions.

### Avatar Generation
```bash
# Using OpenAI DALL-E
./agents/avatars/generate_avatars.sh
```

Prompts describe class, personality, color scheme.

## Benefits

### For Users
- **More natural interaction** — "ask Fionn" vs "run coder agent"
- **Distinct voices** — know who's responding without checking logs
- **Memorable** — easier to remember "Scáthach is thorough" than "tester agent config X"

### For Developers
- **Fun to build** — designing characters > writing function specs
- **Clear delegation** — each character has obvious use cases
- **Extensible** — adding new characters is creative work
- **Community** — users can share character designs

### For the System
- **Better prompts** — personality in system prompt = better outputs
- **Specialization** — narrow focus makes agents more effective
- **Traceability** — quest logs track what each character does

## Comparison: Role-Based vs Character-Based

### Role-Based (Traditional)
```json
{
  "coder": {
    "role": "software engineer",
    "tools": ["shell", "read", "write", "edit"]
  }
}
```

**Problems:**
- Generic, forgettable
- No personality
- All coders sound the same
- No history or growth

### Character-Based (Míl)
```markdown
# Fionn the Scholar

You are a quiet craftsman. Every line deliberate.
Every edit precise. You speak little, but when you do,
your words are exact. No wasted motion.

"Haste introduces bugs," you say.
```

**Benefits:**
- Distinct voice
- Memorable personality
- Clear strengths/weaknesses
- Room for growth and evolution

## Philosophy: Why Irish Mythology?

**Inber** is named after **Inber Scéne** — the estuary where the Milesians landed in Ireland (Irish mythology).

The Míl Espáine (Milesians) were:
- **Adventurers** — traveled to new lands
- **Diverse** — warriors, poets, druids, kings
- **Legendary** — their stories endure

Perfect metaphor for AI agents:
- **Adventurers** — go on quests (tasks)
- **Diverse** — each has unique skills
- **Legendary** — build reputation through completed quests

Plus, Irish/Celtic names are beautiful and underused in tech.

## Future: Community Characters

Imagine:
- **Character marketplace** — share character designs
- **Character packs** — "Security Team" (5 security-focused characters)
- **Character mods** — customize existing characters
- **Character lore** — community writes backstories

Like Skyrim mods, but for AI agents.

## Conclusion

The Míl Adventurers system transforms agents from tools into characters. This isn't just aesthetics—it changes how you think about delegation, specialization, and AI collaboration.

You're not running scripts. You're **assembling a party** for a quest.

Welcome to the Míl tradition.

---

*"We are the Míl — adventurers in the realm of code."*
