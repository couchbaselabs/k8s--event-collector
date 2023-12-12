package config

// EventCollectorConfiguration is the top level config for the event collector
type EventCollectorConfiguration struct {
	Port                  string                          `yaml:"port"`
	BufferSize            int                             `yaml:"bufferSize"`
	DumpCompletionPlugins *CompletionPluginsConfiguration `yaml:"dumpCompletionPlugins"`
	EventFilters          []KubernetesResourceFilter      `yaml:"eventFilter"`
	DumpTrigger           *DumpTriggerConfiguration
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
	APIVersion string `yaml:"apiVersion"`
	Resource   string
	Labels     map[string]string
}

// DumpTriggerConfiguration is a config for triggering automated dumps
type DumpTriggerConfiguration struct {
	EventType    string
	EventFilters []KubernetesResourceFilter
}
