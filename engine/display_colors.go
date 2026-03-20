package engine

// ANSI color helpers
const (
	Reset   = "\033[0m"
	Dim     = "\033[2m"
	Bold    = "\033[1m"
	Italic  = "\033[3m"
	Cyan    = "\033[36m"
	Magenta = "\033[35m"
	Yellow  = "\033[33m"
	Green   = "\033[32m"
	Red     = "\033[31m"
	Blue    = "\033[34m"
)

// Keep lowercase aliases for internal use
const (
	reset   = Reset
	dim     = Dim
	bold    = Bold
	italic  = Italic
	cyan    = Cyan
	magenta = Magenta
	yellow  = Yellow
	green   = Green
	red     = Red
	blue    = Blue
)