package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/spacemeshos/go-spacemesh/metrics"
)

const (
	namespace = "activation"
)

var PostDuration = metrics.NewGauge(
	"post_duration",
	namespace,
	"duration of last PoST in nanoseconds",
	[]string{},
).WithLabelValues()

var PoetPowDuration = metrics.NewGauge(
	"poet_pow_duration",
	namespace,
	"duration of last PoET Proof of Work in nanoseconds",
	[]string{},
).WithLabelValues()

var PostVerificationQueue = metrics.NewGauge(
	"post_verification_waiting_total",
	namespace,
	"the number of POSTs waiting to be verified",
	[]string{},
).WithLabelValues()

var (
	publishWindowLatency = metrics.NewHistogramWithBuckets(
		"publish_window_seconds",
		namespace,
		"how much time in seconds you have before window for poet registrations closes",
		[]string{"condition"},
		prometheus.ExponentialBuckets(1, 2, 10),
	)
	PublishOntimeWindowLatency = publishWindowLatency.WithLabelValues("ontime")
	PublishLateWindowLatency   = publishWindowLatency.WithLabelValues("late")
)

var PostVerificationLatency = metrics.NewHistogramWithBuckets(
	"post_verification_seconds",
	namespace,
	"post verification in seconds",
	[]string{},
	prometheus.ExponentialBuckets(1, 2, 20),
).WithLabelValues()

var AtxWriteTime = metrics.NewSimpleCounter(
	namespace,
	"atx_write_time",
	"time spent writing atxs in storeAtx",
)

var AtxWriteTimeHist = metrics.NewHistogramNoLabel(
	"atx_write_time_hist",
	namespace,
	"time spent writing atxs (histogram)",
	prometheus.ExponentialBuckets(0.01, 10, 5),
)
