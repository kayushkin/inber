package main

import (
	"net/http"

	"github.com/kayushkin/llm-bridge/msg"
	"github.com/kayushkin/llm-bridge/servicesettings"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// serviceName is inber-server's name in its own settings description.
const serviceName = "inber-server"

// ownedEnvironmentVariablePrefixes is empty. Every variable inber-server reads
// is a name other programs read too (AUTH_STORE_URL, NATS_URL, OPENCLAW_URL…),
// so owning any of them would stop the server over a variable set for someone
// else.
var ownedEnvironmentVariablePrefixes = []string{}

// Keys of the settings, as GET /settings names them.
const (
	settingAuthStoreURL   = "auth_store_url"
	settingAuthStoreToken = "auth_store_token"
	settingNatsURL        = "nats_url"
	settingBusToken       = "bus_token"
	settingOpenClawURL    = "openclaw_url"
	settingOpenClawToken  = "openclaw_token"
	settingAgentStorePath = "agent_store_path"
	settingLogstackURL    = "logstack_url"
	settingBlueprint      = "blueprint"
	settingPinchtabURL    = "pinchtab_url"
	settingPinchtabToken  = "pinchtab_token"
	settingBraveAPIKey    = "brave_api_key"
	settingSchedulerURL   = "scheduler_url"
	settingSchedulerToken = "scheduler_token"
	settingAnthropicKey   = "anthropic_api_key"
	settingOpenAIKey      = "openai_api_key"
	settingGoogleKey      = "google_api_key"
	settingOpenRouterKey  = "openrouter_api_key"
)

// defaultAuthStoreURL is where auth-store is asked for the Anthropic
// credential when AUTH_STORE_URL is unset.
const defaultAuthStoreURL = "http://127.0.0.1:8303"

// defaultNatsURL is the bus the server joins when neither NATS_URL nor the
// config file's nats_url names one.
const defaultNatsURL = "nats://localhost:4222"

// settingDefinitions declares every environment variable cmd/inber-server
// reads. The library packages it builds on still read more themselves;
// settings_test.go lists them, and each list names the todo that moves it here.
//
// Nothing is Editable: GET /settings is as open as every other route on :8200,
// so there is no gate to put a write behind.
func settingDefinitions() []servicesettings.Definition {
	return []servicesettings.Definition{
		{Key: settingAuthStoreURL, EnvironmentVariable: "AUTH_STORE_URL", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString, Default: defaultAuthStoreURL,
			Description: "auth-store's base URL. Read once at start, and only when --api-key-from-auth-store is given: the server asks it for the Anthropic credential and fails to start if it cannot."},
		{Key: settingAuthStoreToken, EnvironmentVariable: "AUTH_STORE_TOKEN", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The bearer sent to auth-store when resolving the Anthropic credential. Required when --api-key-from-auth-store is given; unused otherwise."},
		{Key: settingNatsURL, EnvironmentVariable: "NATS_URL", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The NATS server the bus listener joins. Empty means the config file's nats_url, and with no config file " + defaultNatsURL + "."},
		{Key: settingBusToken, EnvironmentVariable: "BUS_TOKEN", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "Token for the bus event publisher. Empty means the config file's bus_token, if any."},
		{Key: settingOpenClawURL, EnvironmentVariable: "OPENCLAW_URL", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString,
			Description: "OpenClaw's base URL. Bus messages whose orchestrator is openclaw are forwarded there. Empty means the config file's openclaw_url, and with neither nothing is forwarded."},
		{Key: settingOpenClawToken, EnvironmentVariable: "OPENCLAW_TOKEN", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The bearer sent with requests forwarded to OpenClaw. Empty means the config file's openclaw_token, if any."},
		{Key: settingAgentStorePath, EnvironmentVariable: "AGENT_STORE_PATH", Kind: msg.ServiceSettingKindPath, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The agent-store database agents are loaded from, and the one status queries read. Empty means agent-store's default, ~/.config/agent-store/agents.db."},
		{Key: settingLogstackURL, EnvironmentVariable: "LOGSTACK_URL", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString,
			Description: "logstack's base URL. Every session's log entries are also posted there. Empty sends them nowhere else."},
		{Key: settingBlueprint, EnvironmentVariable: "INBER_BLUEPRINT", Kind: msg.ServiceSettingKindBehaviour, ValueType: msg.ServiceSettingValueTypeBoolean, Default: "false",
			Description: "Log a prompt blueprint, and its diff from the last one, on every turn of every session. Takes true or false (1, t and their capitals too); any other value stops the server at start."},
		{Key: settingPinchtabURL, EnvironmentVariable: "PINCHTAB_URL", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString,
			Description: "PinchTab's base URL, which the browser tool drives. Empty means tool-store's default, " + toolstoretools.DefaultPinchtabURL + "."},
		{Key: settingPinchtabToken, EnvironmentVariable: "PINCHTAB_TOKEN", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The bearer the browser tool sends to PinchTab. Empty sends none."},
		{Key: settingBraveAPIKey, EnvironmentVariable: "BRAVE_API_KEY", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The Brave Search API key the web search tool sends. Empty makes every web search answer that the key is not set."},
		{Key: settingSchedulerURL, EnvironmentVariable: "SCHEDULER_URL", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The scheduler's base URL, which the scheduler tool calls. Empty means tool-store's default, " + toolstoretools.DefaultSchedulerURL + "."},
		{Key: settingSchedulerToken, EnvironmentVariable: "SCHEDULER_TOKEN", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The bearer the scheduler tool sends to the scheduler. Empty sends none."},
		{Key: settingAnthropicKey, EnvironmentVariable: "ANTHROPIC_API_KEY", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The Anthropic credential sessions and POST /api/oneshot send with. Ignored when --api-key-from-auth-store is given: auth-store's credential is used instead. Empty leaves sessions to aiauth's profiles and makes /api/oneshot answer 503."},
		{Key: settingOpenAIKey, EnvironmentVariable: "OPENAI_API_KEY", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The OpenAI credential sessions send with. Empty leaves them to aiauth's profiles."},
		{Key: settingGoogleKey, EnvironmentVariable: "GOOGLE_API_KEY", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The Google credential sessions send with. Empty leaves them to aiauth's profiles."},
		{Key: settingOpenRouterKey, EnvironmentVariable: "OPENROUTER_API_KEY", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The OpenRouter credential sessions send with. Empty leaves them to aiauth's profiles."},
	}
}

// newSettingsRegistry builds inber-server's settings from environment.
func newSettingsRegistry(environment servicesettings.Environment) (*servicesettings.Registry, error) {
	return servicesettings.New(serviceName, ownedEnvironmentVariablePrefixes, settingDefinitions(), environment)
}

// settingsHandler serves GET /settings. Only GET: nothing is Editable.
func settingsHandler(registry *servicesettings.Registry) http.Handler {
	return servicesettings.Handler(registry, "/settings")
}
