package evcol

import (
	"context"
	"encoding/json"
	"io"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/watch"

	apiWatch "k8s.io/apimachinery/pkg/watch"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("event-collector")

// FilterFunc is a func type which chooses whether an event should be accepted or not
type FilterFunc func(in *corev1.Event) bool

// ActionFunc takes a certain action based on the event that triggered it
type ActionFunc func(in *corev1.Event)

// EventCollector handles the logic of watching for events, filtering them, storing them in the buffer and taking actions
type EventCollector struct {
	KubeClient kubernetes.Interface
	Buffer     EventBuffer
	Namespace  string

	// FilterFunc is optional and will default to accepting all events if
	// not defined
	FilterFunc FilterFunc

	// ActionFilterFunc is optional and no actions will be triggered if not defined
	ActionFilterFunc FilterFunc
	ActionCallback   ActionFunc

	closeChannel chan bool
}

// Run starts the EventCollector
func (ec *EventCollector) Run() {
	watchTimeout := int64(60 * 15)

	watchFunc := func(_ v1.ListOptions) (apiWatch.Interface, error) {
		return ec.KubeClient.CoreV1().Events(ec.GetNamespace()).Watch(context.Background(), v1.ListOptions{
			TimeoutSeconds: &watchTimeout,
			Watch:          true,
		})
	}

	watcher, err := watch.NewRetryWatcher("1", &cache.ListWatch{WatchFunc: watchFunc})

	if err != nil {
		panic(err)
	}

	log.Info("Watcher created, starting event collection")

	ec.closeChannel = make(chan bool, 2)
	for {
		stop := false

		select {
		case event, ok := <-watcher.ResultChan():
			stop = !ec.handleEventReceived(event, ok)
		case <-ec.closeChannel:
			stop = true
		}

		if stop {
			ec.closeChannel = nil
			break
		}
	}

}

// handleEventReceived returns true if it can continue and false if not
func (ec *EventCollector) handleEventReceived(event apiWatch.Event, ok bool) bool {
	if !ok {
		log.Info("WARN, Watcher channel is closed")
		return false
	}

	e, ok := event.Object.(*corev1.Event)

	if !ok {
		log.Info("WARN, Type Mismatch")
		return true
	}

	if ec.FilterFunc != nil && !ec.FilterFunc(e) {
		return true
	}

	ec.Buffer.Add(e)
	log.Info("Event added", "resource", e.Name, "msg", e.Message)

	if ec.ActionFilterFunc != nil && ec.ActionFilterFunc(e) {
		if ec.ActionCallback != nil {
			ec.ActionCallback(e)
		}
	}

	return true
}

// Stop will stop the event collector.
func (ec *EventCollector) Stop() {
	if ec.closeChannel != nil {
		ec.closeChannel <- true
		close(ec.closeChannel)
	}
}

// Stash writes out the current buffer to the provided writer
func (ec *EventCollector) Stash(w io.Writer) error {
	tmpBuff := make([]*corev1.Event, 0, ec.Buffer.Size())

	ec.Buffer.Do(func(e *corev1.Event) {
		tmpBuff = append(tmpBuff, e)
	})

	encoder := json.NewEncoder(w)
	err := encoder.Encode(tmpBuff)

	if err != nil {
		log.Error(err, "Failed to write entries")
		return err
	}

	return nil
}

// GetNamespace gets the namespace the collector is running in
func (ec *EventCollector) GetNamespace() string {
	if ec.Namespace == "" {
		return "default"
	}

	return ec.Namespace
}
