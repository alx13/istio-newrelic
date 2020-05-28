package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/gorilla/mux"

	jaeger "github.com/jaegertracing/jaeger/swagger-gen/models"
	log "github.com/sirupsen/logrus"
)

type config struct {
	Port             int     `env:"PORT" envDefault:"8080"`
	NewRelicAPIKey   string  `env:"NEW_RELIC_API_KEY,required"`
	NewRelicTraceURL url.URL `env:"NEW_RELIC_TRACE_URL,required"`
}

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: false,
	})
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func getNewRelicHandler(cfg config) func(http.ResponseWriter, *http.Request) {
	client := &http.Client{
		Transport: &http.Transport{},
		Timeout:   20 * time.Second,
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var spans []jaeger.Span
		err := json.NewDecoder(r.Body).Decode(&spans)
		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.WithFields(log.Fields{
			"Headers": r.Header,
			"Body":    spans,
		}).Info("Got request")

		payload, err := json.Marshal(spans)
		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		req, err := http.NewRequest(
			"POST",
			cfg.NewRelicTraceURL.String(),
			bytes.NewBuffer(payload),
		)

		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		copyHeader(req.Header, r.Header)
		req.Header.Set("Host", cfg.NewRelicTraceURL.Host)
		req.Header.Set("Api-Key", cfg.NewRelicAPIKey)
		req.Header.Set("Data-Format", "zipkin")
		req.Header.Set("Data-Format-Version", "2")

		log.WithFields(log.Fields{
			"Headers": req.Header,
		}).Infof("Sending request to NR")

		res, err := client.Do(req)
		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.WithFields(log.Fields{
			"Headers":    req.Header,
			"StatusCode": res.StatusCode,
		}).Infof("Got Response from NR")

		copyHeader(w.Header(), res.Header)
		w.WriteHeader(res.StatusCode)
		defer res.Body.Close()
		io.Copy(w, res.Body)
	}
}

func main() {
	var cfg config
	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
	}

	newRelicHandler := getNewRelicHandler(cfg)

	r := mux.NewRouter()
	r.HandleFunc("/api/v2/spans", newRelicHandler).
		Methods("POST").
		Headers("Content-Type", "application/json")

	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}).Methods("GET")

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
