package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type Exporter struct {
	Addr         string
	namespace    string
	duration     prometheus.Gauge
	scrapeErrors prometheus.Counter
	totalScrapes prometheus.Counter
	metrics      map[string]*prometheus.GaugeVec
	metricsMtx   sync.RWMutex
	sync.RWMutex
}

type scrapeResult struct {
	Name   string
	Value  float64
	Type   string
	Status string
}

func NewSwiftExporter(addr string) (*Exporter, error) {
	log.Debug("Creating exporter")
	exp := &Exporter{Addr: addr, namespace: "swift"}

	if err := exp.Ping(); err != nil {
		return exp, err
	}

	exp.initGauges()

	return exp, nil
}

func (exp *Exporter) Ping() error {
	return nil
}

func (exp *Exporter) initGauges() {
	log.Debug("initGauges")
	exp.metrics = map[string]*prometheus.GaugeVec{}
	exp.metrics["async"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "async",
		Help:      "async metric",
	}, []string{"status"})
	exp.metrics["replication_time"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "replication_time",
		Help:      "replication_time metric",
	}, []string{"type"})
	exp.metrics["replication_last"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "replication_last",
		Help:      "replication_last metric",
	}, []string{"type"})
	exp.metrics["replication_stats"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "replication_stats",
		Help:      "replication_stats metric",
	}, []string{"type", "status"})
	exp.metrics["updater_sweep"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "updater_sweep",
		Help:      "updater_sweep metric",
	}, []string{"type"})
	exp.metrics["expirer_expiration_pass"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "expirer_expiration_pass",
		Help:      "expirer_expiration_pass metric",
	}, []string{"type"})
	exp.metrics["expirer_expired_last_pass"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "expired_last_pass",
		Help:      "expired_last_pass metric",
	}, []string{"type"})
	exp.metrics["quarantined"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "quarantined",
		Help:      "quarantined metric",
	}, []string{"type"})
	exp.duration = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "exporter_last_scrape_duration_seconds",
		Help:      "The last scrape duration",
	})
	exp.totalScrapes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: exp.namespace,
		Name:      "exporter_scrapes_total",
		Help:      "Current total redis scrapes",
	})
	exp.scrapeErrors = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: exp.namespace,
		Name:      "exporter_last_scrape_error",
		Help:      "The last scrape error status",
	})
}

func (exp *Exporter) Collect(ch chan<- prometheus.Metric) {
	scrapes := make(chan scrapeResult)

	exp.Lock()
	defer exp.Unlock()

	exp.initGauges()
	go exp.scrape(scrapes)
	exp.setMetrics(scrapes)

	ch <- exp.duration
	ch <- exp.totalScrapes
	ch <- exp.scrapeErrors
	exp.collectMetrics(ch)
}

func (exp *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range exp.metrics {
		m.Describe(ch)
	}

	ch <- exp.duration.Desc()
	ch <- exp.totalScrapes.Desc()
	ch <- exp.scrapeErrors.Desc()
}

func (exp *Exporter) scrape(scrapes chan<- scrapeResult) {
	defer close(scrapes)
	now := time.Now().UnixNano()
	exp.totalScrapes.Inc()

	exp.scrapeAsync(scrapes)
	exp.scrapeReplication(scrapes)
	exp.scrapeUpdater(scrapes)
	exp.scrapeExpirer(scrapes)
	exp.scrapeQuarantined(scrapes)
	//exp.scrapeVersion(scrapes)

	exp.duration.Set(float64(time.Now().UnixNano()-now) / 1000000000)
}

func (exp *Exporter) setMetrics(scrapes <-chan scrapeResult) {
	for scr := range scrapes {
		name := scr.Name
		if _, ok := exp.metrics[name]; !ok {
			exp.metricsMtx.Lock()
			exp.metrics[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: exp.namespace,
				Name:      name,
				Help:      name + " metric", // needs to be set for prometheus >= 2.3.1
			}, []string{"type", "status"})
			exp.metricsMtx.Unlock()
		}
		var labels prometheus.Labels = map[string]string{}
		if len(scr.Type) > 0 {
			labels["type"] = scr.Type
		}
		if len(scr.Status) > 0 {
			labels["status"] = scr.Status
		}
		exp.metrics[name].With(labels).Set(float64(scr.Value))
	}
}

func (exp *Exporter) collectMetrics(metrics chan<- prometheus.Metric) {
	for _, m := range exp.metrics {
		m.Collect(metrics)
	}
}

func (exp *Exporter) request(method string) (map[string]interface{}, error) {
	url := exp.Addr + "/recon/" + method
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&result)
	return result, err
}

func (exp *Exporter) scrapeAsync(scrapes chan<- scrapeResult) {
	res, err := exp.request("async")
	if err != nil {
		log.Error(err)
		return
	}
	if pending, ok := res["async_pending"].(float64); ok {
		scrapes <- scrapeResult{Name: "async", Value: pending, Status: "pending"}
	}
}
func (exp *Exporter) scrapeReplication(scrapes chan<- scrapeResult) {
	var types = []string{"container", "account", "object"}
	for _, t := range types {
		res, err := exp.request("replication/" + t)
		if err != nil {
			log.Error(err)
			return
		}
		if replication_time, ok := res["replication_time"].(float64); ok {
			scrapes <- scrapeResult{Name: "replication_time", Value: replication_time, Type: t}
		}
		if replication_last, ok := res["replication_last"].(float64); ok {
			scrapes <- scrapeResult{Name: "replication_last", Value: replication_last, Type: t}
		}
		if replication_stats, ok := res["replication_stats"].(map[string]interface{}); ok {
			for status, value := range replication_stats {
				if status == "failure_nodes" {
					continue
				}
				if v, ok := value.(float64); ok {
					scrapes <- scrapeResult{Name: "replication_stats", Value: v, Type: t, Status: status}
				}
			}
		}
	}
}
func (exp *Exporter) scrapeUpdater(scrapes chan<- scrapeResult) {
	var types = []string{"container", "object"}
	for _, t := range types {
		res, err := exp.request("updater/" + t)
		if err != nil {
			log.Error(err)
			return
		}
		if sweep, ok := res[t+"_updater_sweep"].(float64); ok {
			scrapes <- scrapeResult{Name: "updater_sweep", Value: sweep, Type: t}
		}
	}
}
func (exp *Exporter) scrapeExpirer(scrapes chan<- scrapeResult) {
	var types = []string{"object"}
	for _, t := range types {
		res, err := exp.request("expirer/" + t)
		if err != nil {
			log.Error(err)
			return
		}
		if expiration_pass, ok := res[t+"_expiration_pass"].(float64); ok {
			scrapes <- scrapeResult{Name: "expirer_expiration_pass", Value: expiration_pass, Type: t}
		}
		if expired_last_pass, ok := res["expired_last_pass"].(float64); ok {
			scrapes <- scrapeResult{Name: "expirer_expired_last_pass", Value: expired_last_pass, Type: t}
		}
	}
}
func (exp *Exporter) scrapeQuarantined(scrapes chan<- scrapeResult) {
	res, err := exp.request("quarantined")
	if err != nil {
		log.Error(err)
		return
	}
	var types = []string{"container", "account", "object"}
	for _, t := range types {
		if v, ok := res[t+"s"].(float64); ok {
			scrapes <- scrapeResult{Name: "quarantined", Value: v, Type: t}
		}
	}
}
