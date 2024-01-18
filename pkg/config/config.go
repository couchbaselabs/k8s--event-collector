package config

// EventCollectorConfiguration is the top level config for the event collector
type EventCollectorConfiguration struct {
	Port                  string                          `yaml:"port"`
	BufferSize            int                             `yaml:"bufferSize"`
	DumpCompletionPlugins *CompletionPluginsConfiguration `yaml:"dumpCompletionPlugins"`
	EventFilters          []KubernetesResourceFilter      `yaml:"eventFilter"`
	DumpOnWarnings        bool                            `yaml:"dumpOnWarningEvents"`
	DumpTrigger           *DumpTriggerConfiguration       `yaml:"dumpTriggers"`
	MaxDumps              int                             `yaml:"maxDumps"`
}

// CompletionPluginsConfiguration is the config for the plugins
type CompletionPluginsConfiguration struct {
	KubernetesEvent *KubernetesEventCompletionConfiguration
}

// KubernetesEventCompletionConfiguration is a config for event completion plugins
type KubernetesEventCompletionConfiguration struct {
	Enabled bool
}

// KubernetesResourceFilter is a simple config to filter events based on API version, resource kind and/or labels
type KubernetesResourceFilter struct {
	APIVersion string            `yaml:"apiVersion"`
	Resource   string            `yaml:"resource"`
	Labels     map[string]string `yaml:"labels"`
}

// DumpTriggerConfiguration is a config for triggering automated dumps
type DumpTriggerConfiguration struct {
	EventType    string
	EventFilters []KubernetesResourceFilter
}
