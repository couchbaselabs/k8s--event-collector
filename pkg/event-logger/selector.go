package elogger

import (
	"sort"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// AnySelector is a selector that will select if any of the requirements are met
type AnySelector []labels.Requirement

// Label type
type Label struct {
	Key, Value string
}

// Matches checks wheter the labels match any of the selectors requirements
func (s AnySelector) Matches(l labels.Labels) bool {
	for ix := range s {
		if matches := s[ix].Matches(l); matches {
			return true
		}
	}
	return false
}

// Add adds a requirement to the list
func (s *AnySelector) Add(reqs ...labels.Requirement) {
	*s = append(*s, reqs...)
}

// AnySelectorFromLabels creates an AnySelector from a slice of labels, where the
// requirements are created to match each label
func AnySelectorFromLabels(ls []Label) AnySelector {
	requirements := make([]labels.Requirement, 0, len(ls))
	for _, l := range ls {
		r, err := labels.NewRequirement(l.Key, selection.Equals, []string{l.Value})
		if err != nil {
			continue
		}
		requirements = append(requirements, *r)
	}
	// sort to have deterministic string representation
	sort.Sort(labels.ByKey(requirements))
	return AnySelector(requirements)
}
