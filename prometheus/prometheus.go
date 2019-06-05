// Copyright 2017 Pilosa Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pilosa/pilosa/logger"
	"github.com/pilosa/pilosa/stats"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// namespace is prepended to each metric event name with "_"
	namespace = "pilosa"
)

// Ensure client implements interface.
var _ stats.StatsClient = &prometheusClient{}

// prometheusClient represents a Prometheus implementation of pilosa.statsClient.
type prometheusClient struct {
	tags        []string
	logger      logger.Logger
	mu          sync.Mutex
	counters    map[string]prometheus.Counter
	counterVecs map[string]*prometheus.CounterVec
}

// NewPrometheusClient returns a new instance of StatsClient.
func NewPrometheusClient() (*prometheusClient, error) {
	return &prometheusClient{
		logger:      logger.NopLogger,
		counters:    make(map[string]prometheus.Counter),
		counterVecs: make(map[string]*prometheus.CounterVec),
	}, nil
}

// Open no-op to satisfy interface
func (c *prometheusClient) Open() {}

// Close no-op to satisfy interface
func (c *prometheusClient) Close() error {
	return nil
}

// Tags returns a sorted list of tags on the client.
func (c *prometheusClient) Tags() []string {
	return c.tags
}

// labels returns an instance of prometheus.Labels with the value of the set tags.
func (c *prometheusClient) labels() prometheus.Labels {
	return tagsToLabels(c.tags)
}

// WithTags returns a new client with additional tags appended.
func (c *prometheusClient) WithTags(tags ...string) stats.StatsClient {
	return &prometheusClient{
		tags:        unionStringSlice(c.tags, tags),
		logger:      c.logger,
		mu:          c.mu,
		counters:    c.counters,
		counterVecs: c.counterVecs,
	}
}

// Count tracks the number of times something occurs per second.
func (c *prometheusClient) Count(name string, value int64, rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var counter prometheus.Counter
	labels := c.labels()
	opts := prometheus.CounterOpts{
		Namespace: namespace,
		Name:      name,
	}
	if len(labels) == 0 {
		if counter, ok := c.counters[name]; !ok {
			counter = prometheus.NewCounter(opts)
			c.counters[name] = counter
			prometheus.MustRegister(counter)
		}
	} else {
		var counterVec *prometheus.CounterVec
		counterVec, ok := c.counterVecs[name]
		if !ok {
			counterVec = prometheus.NewCounterVec(
				opts,
				labelKeys(labels),
			)
			c.counterVecs[name] = counterVec
			prometheus.MustRegister(counterVec)
		}
		var err error
		counter, err = counterVec.GetMetricWith(labels)
		if err != nil {
			c.logger.Printf("counterVec.GetMetricWith error: %s", err)
		}
	}
	if value == 1 {
		counter.Inc()
	} else {
		counter.Add(float64(value))
	}
}

// CountWithCustomTags tracks the number of times something occurs per second with custom tags.
func (c *prometheusClient) CountWithCustomTags(name string, value int64, rate float64, t []string) {
	c.WithTags(append(c.tags, t...)...).Count(name, value, rate)
}

// Gauge sets the value of a metric.
func (c *prometheusClient) Gauge(name string, value float64, rate float64) {
}

// Histogram tracks statistical distribution of a metric.
func (c *prometheusClient) Histogram(name string, value float64, rate float64) {
}

// Set tracks number of unique elements.
func (c *prometheusClient) Set(name string, value string, rate float64) {
}

// Timing tracks timing information for a metric.
func (c *prometheusClient) Timing(name string, value time.Duration, rate float64) {
}

// SetLogger sets the logger for client.
func (c *prometheusClient) SetLogger(logger logger.Logger) {
	c.logger = logger
}

// unionStringSlice returns a sorted set of tags which combine a & b.
func unionStringSlice(a, b []string) []string {
	// Sort both sets first.
	sort.Strings(a)
	sort.Strings(b)

	// Find size of largest slice.
	n := len(a)
	if len(b) > n {
		n = len(b)
	}

	// Exit if both sets are empty.
	if n == 0 {
		return nil
	}

	// Iterate over both in order and merge.
	other := make([]string, 0, n)
	for len(a) > 0 || len(b) > 0 {
		if len(a) == 0 {
			other, b = append(other, b[0]), b[1:]
		} else if len(b) == 0 {
			other, a = append(other, a[0]), a[1:]
		} else if a[0] < b[0] {
			other, a = append(other, a[0]), a[1:]
		} else if b[0] < a[0] {
			other, b = append(other, b[0]), b[1:]
		} else {
			other, a, b = append(other, a[0]), a[1:], b[1:]
		}
	}
	return other
}

func tagsToLabels(tags []string) (labels prometheus.Labels) {
	labels = make(prometheus.Labels)
	for _, tag := range tags {
		tagParts := strings.SplitAfterN(tag, ":", 2)
		if len(tagParts) != 2 {
			// only process tags in "key:value" form
			continue
		}
		labels[tagParts[0][0:len(tagParts[0])-1]] = tagParts[1]
	}
	return labels
}

func labelKeys(labels prometheus.Labels) (keys []string) {
	keys = make([]string, len(labels))
	i := 0
	for k := range labels {
		keys[i] = k
		i++
	}
	return keys
}
