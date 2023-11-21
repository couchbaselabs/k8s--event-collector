package main

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"

	elogger "github.com/couchbase/k8s-event-logger/pkg/event-logger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type EventLoggerConfiguration struct {
	Port       string `yaml:"port" env:"PORT" env-default:"8080"`
	BufferSize int    `yaml:"bufferSize" env:"BUF_SIZE" env-default:"1000"`
}

var cfg EventLoggerConfiguration

var log = logf.Log.WithName("main")

// main initializes the system then starts a HTTPS server to process requests.
func main() {
	// Create Client
	kubeClient, err := getClient()

	if err != nil {
		panic(err)
	}

	// Load config
	var cfg EventLoggerConfiguration
	err = cleanenv.ReadConfig("config.yml", &cfg)
	if err != nil {
		log.Error(err, "failed to read config file")
		cleanenv.ReadEnv(&cfg)
	}
	log.Info(fmt.Sprintf("Config: %+v", cfg))

	buff := elogger.NewRingEventBuffer(cfg.BufferSize)

	eventlogger := elogger.EventLogger{
		Buffer:           buff,
		KubeClient:       kubeClient,
		ActionFilterFunc: actionFilter,
	}

	dumpServer := elogger.NewDumpServer(&eventlogger)

	eventlogger.ActionCallback = func(in *corev1.Event) {
		dumpServer.CreateBufferDump()
	}

	go func() {
		dumpServer.Run(cfg.Port)
	}()

	eventlogger.Run()
}

func getClient() (kubernetes.Interface, error) {
	kubeConfig, err := rest.InClusterConfig()

	if err != nil {
		return nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)

	if err != nil {
		return nil, err

	}

	return kubeClient, nil
}

func actionFilter(e *corev1.Event) bool {
	return e.Type == "Warning"
}
