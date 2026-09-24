package main

import (
	"net/http"

	"github.com/kayushkin/llm-bridge/msg"
	"github.com/kayushkin/llm-bridge/servicesettings"
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
)

// defaultAuthStoreURL is where auth-store is asked for the Anthropic
// credential when AUTH_STORE_URL is unset.
const defaultAuthStoreURL = "http://127.0.0.1:8303"

// defaultNatsURL is the bus the server joins when neither NATS_URL nor the
// config file's nats_url names one.
const defaultNatsURL = "nats://localhost:4222"

// settingDefinitions declares every environment variable cmd/inber-server
// reads. The library packages it builds on still read eight more themselves;
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
