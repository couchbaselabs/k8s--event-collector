package evcol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stest "k8s.io/client-go/testing"
)

func getMockClient() (kubernetes.Interface, *watch.FakeWatcher) {
	mockClient := fake.NewSimpleClientset()

	watcher := watch.NewFake()
	mockClient.PrependWatchReactor("events", k8stest.DefaultWatchReactor(watcher, nil))
	return mockClient, watcher
}

func TestWatchingEvents(t *testing.T) {
	mockClient, watcher := getMockClient()
	defer watcher.Stop()

	collector := EventCollector{
		KubeClient: mockClient,
		Buffer:     NewRingEventBuffer(5),
	}

	go func() {
		collector.Run()
	}()

	numEvents := 3
	for i := 0; i < numEvents; i++ {
		e := createEvent()
		watcher.Add(&e)
	}

	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	if collector.Buffer.Size() != numEvents {
		t.Error("Expected an event to be buffereed")
	}
}

func TestCollectorFilterFunc(t *testing.T) {
	mockClient, watcher := getMockClient()
	defer watcher.Stop()

	collector := EventCollector{
		KubeClient: mockClient,
		Buffer:     NewRingEventBuffer(5),
		FilterFunc: func(in *corev1.Event) bool {
			return in.GetName() == "Log"
		},
	}

	go func() {
		collector.Run()
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
	collector.Stop()

	if collector.Buffer.Size() != numLogEvents {
		t.Error("Expected an event to be buffereed")
	}
}

func TestCollectorActionFunc(t *testing.T) {
	mockClient, watcher := getMockClient()
	defer watcher.Stop()

	actionCounter := 0

	collector := EventCollector{
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
		collector.Run()
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
	collector.Stop()

	if actionCounter != numActionEvents {
		t.Error("Expected an event to be buffereed")
	}
}

func TestStash(t *testing.T) {
	mockClient, watcher := getMockClient()
	defer watcher.Stop()

	collector := EventCollector{
		KubeClient: mockClient,
		Buffer:     NewRingEventBuffer(5),
	}

	go func() {
		collector.Run()
	}()

	numEvents := 1
	var events []corev1.Event
	for i := 0; i < numEvents; i++ {
		e := createEvent()
		events = append(events, e)
		watcher.Add(&e)
	}

	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	var builder strings.Builder

	collector.Stash(&builder)
	var readEvents []corev1.Event
	json.Unmarshal([]byte(builder.String()), &readEvents)
	if !reflect.DeepEqual(readEvents, events) {
		t.Errorf("Expected sent events to match stashed events")
	}
}

func TestHandleTypeMismatches(t *testing.T) {
	mockClient := fake.NewSimpleClientset()

	watcher := watch.NewFake()
	mockClient.PrependWatchReactor("events", k8stest.DefaultWatchReactor(watcher, nil))

	collector := EventCollector{
		KubeClient: mockClient,
		Buffer:     NewRingEventBuffer(5),
	}

	go func() {
		collector.Run()
	}()

	defer collector.Stop()
	for i := 0; i < 2; i++ {
		p := corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				UID:             types.UID(rand.String(36)),
				ResourceVersion: "1",
			},
		}
		watcher.Add(&p)
	}

	time.Sleep(100 * time.Millisecond)

	if collector.closeChannel == nil {
		t.Errorf("Collectors close channel should still be open")
	}
}
