module github.com/kayushkin/inber

go 1.24.0

require (
	github.com/anthropics/anthropic-sdk-go v1.26.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/joho/godotenv v1.5.1
	github.com/kayushkin/agent-store v0.0.0
	github.com/kayushkin/agentkit v0.0.0-20260301045703-8024de8a359f
	github.com/kayushkin/logstack v0.0.0-20260304030639-2b277d8d231e
	github.com/kayushkin/model-store v0.0.0-20260307230928-77f7530097d2
	github.com/mattn/go-sqlite3 v1.14.34
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kayushkin/aiauth v0.0.0 // indirect
	github.com/kayushkin/forge v0.0.0-20260308213252-e9c2836d5716 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.46.1 // indirect
)

replace github.com/kayushkin/aiauth => ../aiauth

replace github.com/kayushkin/model-store => ../model-store

replace github.com/kayushkin/agent-store => ../agent-store

replace github.com/kayushkin/forge => ../forge
