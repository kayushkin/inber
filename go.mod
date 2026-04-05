module github.com/kayushkin/inber

go 1.24.0

require (
	github.com/anthropics/anthropic-sdk-go v1.27.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/joho/godotenv v1.5.1
	github.com/kayushkin/agent-store v0.0.0-20260405014345-d199a8b99810
	github.com/kayushkin/agentkit v0.0.0-20260405052600-a2f937bf60ee
	github.com/kayushkin/bus v0.0.0-20260324013021-8010a5dab1d6
	github.com/kayushkin/forge v0.0.0
	github.com/kayushkin/logstack v0.0.0-20260322075744-a4ca356093f8
	github.com/kayushkin/model-store v0.0.0-20260307230928-77f7530097d2
	github.com/spf13/cobra v1.10.2
	modernc.org/sqlite v1.46.1
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kayushkin/aiauth v0.0.0 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.37 // indirect
	github.com/nats-io/nats.go v1.49.0 // indirect
	github.com/nats-io/nkeys v0.4.12 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/kayushkin/aiauth => ../aiauth

replace github.com/kayushkin/model-store => ../model-store

replace github.com/kayushkin/agent-store => ../agent-store

replace github.com/kayushkin/forge => ../forge

replace github.com/kayushkin/agentkit => ../agentkit

replace github.com/kayushkin/bus => ../bus
