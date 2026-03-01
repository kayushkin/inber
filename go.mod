module github.com/kayushkin/inber

go 1.24.0

require (
	github.com/anthropics/anthropic-sdk-go v1.26.0
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/kayushkin/agentkit v0.0.0-20260301000152-f583ad0a0625
	github.com/kayushkin/aiauth v0.0.0-20260226191106-26d44eea610e
	github.com/mattn/go-sqlite3 v1.14.34
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	golang.org/x/sync v0.16.0 // indirect
)

replace github.com/kayushkin/aiauth => ../aiauth
