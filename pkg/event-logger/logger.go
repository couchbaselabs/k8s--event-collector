package elogger

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

var log = logf.Log.WithName("event-logger")

// FilterFunc is a func type which chooses whether an event should be accepted or not
type FilterFunc func(in *corev1.Event) bool

// ActionFunc takes a certain action based on the event that triggered it
type ActionFunc func(in *corev1.Event)

// EventLogger handles the logic of watching for events, filtering them, storing them in the buffer and taking actions
type EventLogger struct {
	KubeClient kubernetes.Interface

	Buffer EventBuffer

	// FilterFunc is optional and will default to accepting all events if
	// not defined
	FilterFunc FilterFunc

	// ActionFilterFunc is optional and no actions will be triggered if not defined
	ActionFilterFunc FilterFunc

	ActionCallback ActionFunc

	closeChannel chan bool
}

// Run starts the EventLogger
func (el *EventLogger) Run() {
	watchTimeout := int64(60 * 5)

	watchFunc := func(_ v1.ListOptions) (apiWatch.Interface, error) {
		// Return watcher with 1 min timeout
		return el.KubeClient.CoreV1().Events("default").Watch(context.Background(), v1.ListOptions{
			TimeoutSeconds: &watchTimeout,
			Watch:          true,
		})
	}

	watcher, err := watch.NewRetryWatcher("1", &cache.ListWatch{WatchFunc: watchFunc})

	if err != nil {
		panic(err)
	}

	log.Info("Watcher created, starting event logging")

	el.closeChannel = make(chan bool, 2)
	for {
		stop := false

		select {
		case event, ok := <-watcher.ResultChan():
			stop = !el.handleEventReceived(event, ok)
		case <-el.closeChannel:
			stop = true
		}

		if stop {
			el.closeChannel = nil
			break
		}
	}

}

func (el *EventLogger) handleEventReceived(event apiWatch.Event, ok bool) bool {
	if !ok {
		log.Info("WARN, Watcher channel is closed")
		return false
	}

	e, ok := event.Object.(*corev1.Event)

	if !ok {
		log.Info("WARN, Type Mismatch")
		return true
	}

	if el.FilterFunc != nil && !el.FilterFunc(e) {
		return true
	}

	el.Buffer.Add(e)
	log.Info("Event added", "resource", e.Name, "msg", e.Message)

	if el.ActionFilterFunc != nil && el.ActionFilterFunc(e) {
		if el.ActionCallback != nil {
			el.ActionCallback(e)
		}
	}

	return true
}

// Stop will stop the event logger.
func (el *EventLogger) Stop() {
	if el.closeChannel != nil {
		el.closeChannel <- true
		close(el.closeChannel)
	}
}

func (el *EventLogger) dump(w io.Writer) error {
	tmpBuff := make([]*corev1.Event, 0, el.Buffer.Size())

	el.Buffer.Do(func(e *corev1.Event) {
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

func (el *EventLogger) getDumpLocation(dumpName string) string {
	return dumpDir + dumpName + dumpFileExtension
}
