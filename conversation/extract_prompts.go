package conversation

const extractionPrompt = `Extract any facts, decisions, preferences, or important context from this exchange worth remembering long-term.

For each item, provide:
- content: the fact/decision (concise, 1-2 sentences max)
- importance: 0.0-1.0 (0.3=minor, 0.5=moderate, 0.7=important, 0.9=critical)
- tags: relevant keywords (e.g., ["coding", "preference"], ["decision", "architecture"])

Guidelines:
- Only extract genuinely useful information worth remembering across sessions
- Skip trivial greetings, confirmations, or transient information
- Focus on: decisions made, preferences stated, facts learned, problems solved
- Be concise - each memory should be self-contained and clear
- If nothing worth remembering, return empty array

Response format: JSON array of {content, importance, tags[]}

Example:
[
  {
    "content": "User prefers using Go for backend services, values simplicity over abstraction",
    "importance": 0.6,
    "tags": ["preference", "golang", "architecture"]
  },
  {
    "content": "Decided to use SQLite for memory storage instead of external DB to minimize dependencies",
    "importance": 0.7,
    "tags": ["decision", "architecture", "database"]
  }
]

Exchange to analyze:
`