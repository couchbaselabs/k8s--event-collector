package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/viper"

	"github.com/couchbase/k8s-event-collector/pkg/config"
	evcol "github.com/couchbase/k8s-event-collector/pkg/event-collector"
	"github.com/couchbase/k8s-event-collector/pkg/plugins"
	"github.com/couchbase/k8s-event-collector/pkg/stashserver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var log = logf.Log.WithName("main")

// main initializes the system then starts a HTTP server to process requests.
func main() {
	// Setup Logging
	logf.SetLogger(zap.New(zap.UseDevMode(false)))

	// Create Client
	kubeClient, err := getKubeClient()

	if err != nil {
		panic(err)
	}

	cfg := loadConfig()

	// Create Buffer
	buff := evcol.NewRingEventBuffer(cfg.BufferSize)

	// Create Event Logger
	ns, _ := getNamespace()
	eventcollector := evcol.EventCollector{
		Buffer:     buff,
		KubeClient: kubeClient,
		Namespace:  ns,
	}
	addFilterFunction(&eventcollector, cfg.EventFilters, kubeClient)
	addActionFunc(&eventcollector, cfg)

	// Create and setup stashServer
	stashServer := stashserver.NewStashServer(&eventcollector, cfg.MaxStashes)
	eventcollector.ActionCallback = func(in *corev1.Event) {
		stashServer.CreateBufferStash()
	}
	plugins.AddPlugins(stashServer, cfg.StashCompletionPlugins, kubeClient)

	// Start Server and Logger
	go func() {
		stashServer.Run(cfg.Port)
	}()

	eventcollector.Run()
}

func addActionFunc(el *evcol.EventCollector, cfg config.EventCollectorConfiguration) {
	if cfg.StashTrigger != nil {
		eventType := cfg.StashTrigger.EventType
		if eventType == "" && cfg.StashTrigger.EventFilters == nil {
			eventType = corev1.EventTypeWarning
		}
		configFilterFunc := createFilterFuncFromConfigFilters(cfg.StashTrigger.EventFilters, el.KubeClient)

		el.ActionFilterFunc = func(in *corev1.Event) bool {
			if eventType != "" && in.Type != eventType {
				return false
			}

			return configFilterFunc(in)
		}
	} else if cfg.StashOnWarnings {
		el.ActionFilterFunc = func(in *corev1.Event) bool {
			return in.Type == corev1.EventTypeWarning
		}
	}
}

func getKubeClient() (kubernetes.Interface, error) {
	kubeConfig, err := getKubeConfig()

	if err != nil {
		return nil, err
	}

	dynamic.NewForConfig(kubeConfig)
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)

	if err != nil {
		return nil, err

	}

	return kubeClient, nil
}

func getKubeConfig() (*rest.Config, error) {
	errs := []error{}
	kubeConfig, err := rest.InClusterConfig()

	if err == nil {
		return kubeConfig, nil
	}
	errs = append(errs, err)

	// If we aren't running in a pod then load kube config
	c := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	kubeConfig, err = c.ClientConfig()
	if err == nil {
		return kubeConfig, nil
	}
	errs = append(errs, err)
	return nil, errors.Join(errs...)
}

func loadConfig() config.EventCollectorConfiguration {
	viper.SetConfigName("config.yaml")
	viper.SetConfigType("yaml")

	// By default look in /etc/eventcollector/ and local directory
	viper.AddConfigPath("/etc/eventcollector/")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()

	viper.SetDefault("bufferSize", 100)
	viper.SetDefault("port", "8080")
	viper.SetDefault("maxStashes", "20")

	if err != nil {
		log.Info("WARN: Failed to read config file", "error", err)
	}

	var cfg config.EventCollectorConfiguration

	err = viper.Unmarshal(&cfg)
	if err != nil {
		log.Error(err, "failed to unmarshall config")
		panic(err)
	}

	log.Info(fmt.Sprintf("Config: %+v", cfg))

	return cfg
}

func addFilterFunction(el *evcol.EventCollector, filters []config.KubernetesResourceFilter, kubeClient kubernetes.Interface) {
	if len(filters) == 0 {
		return
	}

	el.FilterFunc = createFilterFuncFromConfigFilters(filters, kubeClient)
}

func createFilterFuncFromConfigFilters(filters []config.KubernetesResourceFilter, kubeClient kubernetes.Interface) evcol.FilterFunc {
	if len(filters) == 0 {
		return func(in *corev1.Event) bool {
			return true
		}
	}

	selectors := make([]labels.Selector, len(filters))
	for i, f := range filters {
		if len(f.Labels) != 0 {
			sel := labels.SelectorFromSet(labels.Set(f.Labels))
			selectors[i] = sel
		}
	}

	return func(in *corev1.Event) bool {
		for i, f := range filters {
			if f.APIVersion != "" && f.APIVersion != in.InvolvedObject.APIVersion {
				continue
			}

			if f.Resource != "" && f.Resource != in.InvolvedObject.Kind {
				continue
			}

			if sel := selectors[i]; sel != nil {
				switch in.InvolvedObject.Kind {
				case "Pod":
					p, err := kubeClient.CoreV1().Pods(in.Namespace).Get(context.Background(), in.InvolvedObject.Name, metav1.GetOptions{})
					return (err == nil) && sel.Matches(labels.Set(p.Labels))
				case "Deployment":
					d, err := kubeClient.AppsV1().Deployments(in.Namespace).Get(context.Background(), in.InvolvedObject.Name, metav1.GetOptions{})
					return (err == nil) && sel.Matches(labels.Set(d.Labels))
				case "PersistentVolumeClaim":
					pvc, err := kubeClient.CoreV1().PersistentVolumeClaims(in.Namespace).Get(context.Background(), in.InvolvedObject.Name, metav1.GetOptions{})
					return (err == nil) && sel.Matches(labels.Set(pvc.Labels))
				default:
					return false
				}
			}

			return true
		}
		return false
	}
}

func getNamespace() (string, error) {
	b, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")

	if err != nil {
		return "", err
	}

	return string(b), nil
}
