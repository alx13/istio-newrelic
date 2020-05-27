package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/newrelic/go-agent/v3/integrations/nrgorilla"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/gorilla/mux"

	env "github.com/caarlos0/env/v6"
	log "github.com/sirupsen/logrus"
)

type config struct {
	Name               string   `env:"NAME,required"`
	NewRelicLicenseKey string   `env:"NEW_RELIC_LICENSE_KEY,required"`
	Port               int      `env:"PORT" envDefault:"8080"`
	UpstreamURL        *url.URL `env:"UPSTREAM_URL"`
	App                *newrelic.Application
}

func getHandler(cfg config) func(http.ResponseWriter, *http.Request) {
	client := &http.Client{
		Transport: &http.Transport{},
		Timeout:   20 * time.Second,
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txn := newrelic.FromContext(r.Context())

		rDump, err := httputil.DumpRequest(r, false)
		if err != nil {
			log.Error(err)
		}
		log.Infof("Got request\n%s", string(rDump))

		if cfg.UpstreamURL == nil {
			log.Info("Terminating endpoint, sending response")
			fmt.Fprintf(w, "[terminating upstream got %s]", r.URL.Path)
			return
		}

		newURL := *cfg.UpstreamURL
		newURL.Path = r.URL.Path

		req, err := http.NewRequest("GET", newURL.String(), nil)
		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		reqDump, err := httputil.DumpRequestOut(req, false)
		if err != nil {
			log.Error(err)
		}
		log.Infof("Sending request to upstream\n%s", string(reqDump))

		es := newrelic.StartExternalSegment(txn, req)
		res, err := client.Do(req)
		es.End()
		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resDump, err := httputil.DumpResponse(res, true)
		if err != nil {
			log.Error(err)
		}
		log.Infof("Got response from upstream\n%s", string(resDump))

		log.Info("sending response")

		defer res.Body.Close()
		io.Copy(w, res.Body)
	}
}

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: false,
	})
}

func main() {
	var cfg config
	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
	}
	log.Info(cfg)

	app, err := newrelic.NewApplication(
		newrelic.ConfigAppName(cfg.Name),
		newrelic.ConfigLicense(cfg.NewRelicLicenseKey),
		newrelic.ConfigDistributedTracerEnabled(true),
	)
	if err != nil {
		log.Fatal(err)
	}

	cfg.App = app

	r := mux.NewRouter()

	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}).Methods("GET")

	s := r.PathPrefix("/api").Subrouter()
	s.Use(nrgorilla.Middleware(app))
	s.HandleFunc("/", getHandler(cfg)).Methods("GET")

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	log.Infof("Starting server at %s", addr)
	srv := &http.Server{
		Handler:      r,
		Addr:         addr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Error("Can't start server: ", err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Info("shutting down")
	os.Exit(0)
}
