package plugins

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/couchbase/k8s-event-collector/pkg/config"
	"github.com/couchbase/k8s-event-collector/pkg/stashserver"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("completion-plugins")

// AddPlugins parses the config and adds the specified plugins
func AddPlugins(ss *stashserver.StashServer, cfg *config.CompletionPluginsConfiguration, kubeClient kubernetes.Interface) {
	if cfg == nil {
		return
	}

	if ke := cfg.KubernetesEvent; ke != nil {
		if ke.Enabled {
			ss.AddCompletionCallback(func(d *stashserver.Stash) {
				CreateStashEvent(d, kubeClient)
			})
			log.Info("Added Kubernetes Event Completion plugin")
		}
	}
}

func CreateStashEvent(d *stashserver.Stash, c kubernetes.Interface) {
	selfPod := getSelfPod(c)

	if selfPod == nil {
		return
	}

	t := time.Now()

	msg := fmt.Sprintf("Stash %s created", d.Name)
	e := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: selfPod.Name,
			Namespace:    selfPod.GetNamespace(),
		},
		InvolvedObject: v1.ObjectReference{
			APIVersion:      selfPod.APIVersion,
			Kind:            selfPod.Kind,
			Name:            selfPod.Name,
			Namespace:       selfPod.Namespace,
			UID:             selfPod.UID,
			ResourceVersion: selfPod.ResourceVersion,
		},
		Source: v1.EventSource{
			Component: selfPod.Name,
		},
		Type:           v1.EventTypeNormal,
		FirstTimestamp: metav1.Time{Time: t},
		LastTimestamp:  metav1.Time{Time: t},
		Count:          int32(1),
		Message:        msg,
		Reason:         "Stash triggered",
	}

	e, err := c.CoreV1().Events("default").Create(context.TODO(), e, metav1.CreateOptions{})

	if err != nil {
		log.Error(err, "Failed to create K8s stash event")
	}
}

func getNamespace() (string, error) {
	b, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")

	if err != nil {
		return "", err
	}

	return string(b), nil
}

func getSelfPod(c kubernetes.Interface) *v1.Pod {
	podName := os.Getenv("POD_NAME")

	if podName == "" {
		return nil
	}

	ns, err := getNamespace()

	if err != nil {
		return nil
	}

	pod, err := c.CoreV1().Pods(ns).Get(context.TODO(), podName, metav1.GetOptions{})

	if err != nil {
		return nil
	}

	return pod
}
