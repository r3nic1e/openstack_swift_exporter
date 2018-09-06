package main

import (
	"flag"
	"net/http"
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	swiftAddr     = flag.String("swift.addr", "http://127.0.0.1:6000", "Address of swift API")
	showVersion   = flag.Bool("version", false, "Show version information and exit")
	metricPath    = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	listenAddress = flag.String("web.listen-address", ":9500", "Address to listen on for web interface and telemetry.")
	isDebug       = flag.Bool("debug", false, "Output verbose debug information")

	VERSION     = "<<< filled in by build >>>"
	BUILD_DATE  = "<<< filled in by build >>>"
	COMMIT_SHA1 = "<<< filled in by build >>>"
)

func main() {
	flag.Parse()

	log.Printf("Swift Metrics Exporter %s    build date: %s    sha1: %s    Go: %s",
		VERSION, BUILD_DATE, COMMIT_SHA1,
		runtime.Version(),
	)
	if *showVersion {
		return
	}

	if *isDebug {
		log.SetLevel(log.DebugLevel)
		log.Debugln("Enabling debug output")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	exp, err := NewSwiftExporter(*swiftAddr)
	if err != nil {
		log.Fatal(err)
	}

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "swift_exporter_build_info",
		Help: "swift exporter build_info",
	}, []string{"version", "commit_sha", "build_date", "golang_version"})
	buildInfo.WithLabelValues(VERSION, COMMIT_SHA1, BUILD_DATE, runtime.Version()).Set(1)

	prometheus.MustRegister(exp)
	prometheus.MustRegister(buildInfo)
	http.Handle(*metricPath, promhttp.Handler())

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
<html>
<head><title>Swift Exporter v` + VERSION + `</title></head>
<body>
<h1>Swift Exporter ` + VERSION + `</h1>
<p><a href='` + *metricPath + `'>Metrics</a></p>
</body>
</html>
						`))
	})

	log.Printf("Providing metrics at %s%s", *listenAddress, *metricPath)
	log.Printf("Connecting to swift host: %s", *swiftAddr)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
