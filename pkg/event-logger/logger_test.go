package elogger

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stest "k8s.io/client-go/testing"
)

func TestWatchingEvents(t *testing.T) {
	mockClient := fake.NewSimpleClientset()

	watcher := watch.NewFake()
	defer watcher.Stop()
	mockClient.PrependWatchReactor("events", k8stest.DefaultWatchReactor(watcher, nil))

	logger := EventLogger{
		KubeClient: mockClient,
		Buffer:     NewRingEventBuffer(5),
	}

	go func() {
		logger.Run()
	}()

	numEvents := 3
	for i := 0; i < numEvents; i++ {
		e := createEvent()
		watcher.Add(&e)
	}

	time.Sleep(100 * time.Millisecond)
	logger.Stop()

	if logger.Buffer.Size() != numEvents {
		t.Error("Expected an event to be buffereed")
	}
}

func TestLoggerFilterFunc(t *testing.T) {
	mockClient := fake.NewSimpleClientset()

	watcher := watch.NewFake()
	defer watcher.Stop()
	mockClient.PrependWatchReactor("events", k8stest.DefaultWatchReactor(watcher, nil))

	logger := EventLogger{
		KubeClient: mockClient,
		Buffer:     NewRingEventBuffer(5),
		FilterFunc: func(in *corev1.Event) bool {
			return in.GetName() == "Log"
		},
	}

	go func() {
		logger.Run()
	}()

	numLogEvents := 3
	for i := 0; i < numLogEvents; i++ {
		e := createEvent()
		e.SetName("Log")
		watcher.Add(&e)
	}

	numIgnoreEvents := 10
	for i := 0; i < numIgnoreEvents; i++ {
		e := createEvent()
		e.SetName("Ignore")
		watcher.Add(&e)
	}

	time.Sleep(100 * time.Millisecond)
	logger.Stop()

	if logger.Buffer.Size() != numLogEvents {
		t.Error("Expected an event to be buffereed")
	}
}

func TestLoggerActionFunc(t *testing.T) {
	mockClient := fake.NewSimpleClientset()

	watcher := watch.NewFake()
	defer watcher.Stop()
	mockClient.PrependWatchReactor("events", k8stest.DefaultWatchReactor(watcher, nil))

	actionCounter := 0

	logger := EventLogger{
		KubeClient: mockClient,
		Buffer:     NewRingEventBuffer(5),
		ActionFilterFunc: func(in *corev1.Event) bool {
			return in.GetName() == "Action"
		},
		ActionCallback: func(in *corev1.Event) {
			actionCounter++
		},
	}

	go func() {
		logger.Run()
	}()

	numActionEvents := 3
	for i := 0; i < numActionEvents; i++ {
		e := createEvent()
		e.SetName("Action")
		watcher.Add(&e)
	}

	otherEvents := 10
	for i := 0; i < otherEvents; i++ {
		e := createEvent()
		watcher.Add(&e)
	}

	time.Sleep(100 * time.Millisecond)
	logger.Stop()

	if actionCounter != numActionEvents {
		t.Error("Expected an event to be buffereed")
	}
}
