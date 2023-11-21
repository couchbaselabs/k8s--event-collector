package elogger

import (
	"testing"

	"k8s.io/apimachinery/pkg/labels"
)

func TestAnySelectorFromLabels(t *testing.T) {
	ls := []Label{{"app", "nginx"}, {"app", "couchbase"}, {"component", "logger"}}

	s := AnySelectorFromLabels(ls)

	for _, l := range ls {
		if !s.Matches(labels.Set(map[string]string{l.Key: l.Value})) {
			t.Errorf("Selector should match label %v : %v", l.Key, l.Value)
		}
	}
}
