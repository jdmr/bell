package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/natefinch/lumberjack"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var isDev *bool

func main() {
	isDev = flag.Bool("dev", false, "is it running in development mode")
	flag.Parse()

	viper.SetConfigName("bell")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		log.Panicf("Could not load bell.yml configuration file: %v", err)
	}

	// Setup logger
	lumberjackLogrotate := &lumberjack.Logger{
		Filename:   viper.GetString("log.file"),
		MaxSize:    viper.GetInt("log.max-size"),    // Max megabytes before log is rotated
		MaxBackups: viper.GetInt("log.max-backups"), // Max number of old log files to keep
		MaxAge:     viper.GetInt("log.max-age"),     // Max number of days to retain log files
		Compress:   false,
	}

	if *isDev {
		log.SetReportCaller(true)
		log.SetFormatter(&log.TextFormatter{
			ForceColors:     true,
			FullTimestamp:   true,
			TimestampFormat: "2006/01/02 15:04:05",
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				filename := path.Base(f.File)
				return fmt.Sprintf("%s()", f.Function), fmt.Sprintf("\t%s:%d", filename, f.Line)
			},
		})
	} else {
		log.SetFormatter(&log.JSONFormatter{})
	}
	logMultiWriter := io.MultiWriter(os.Stdout, lumberjackLogrotate)
	log.SetOutput(logMultiWriter)
	switch viper.GetString("log.level") {
	case "INFO":
		log.SetLevel(log.InfoLevel)
	case "DEBUG":
		log.SetLevel(log.DebugLevel)
	case "TRACE":
		log.SetLevel(log.TraceLevel)
	default:
		log.SetLevel(log.WarnLevel)
	}

	log.WithFields(log.Fields{
		"Runtime Version": runtime.Version(),
		"Number of CPUs":  runtime.NumCPU(),
		"Arch":            runtime.GOARCH,
	}).Info("Starting bell")

	err = parseSchedule()
	if err != nil {
		log.Fatalf("Could not parse schedule: %v", err)
	}
	// limiter := tollbooth.NewLimiter(1, &limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour})

	r := mux.NewRouter()
	r.Use(loggingMiddleware)
	r.HandleFunc("/api/v1/healthz", getHealthzHandler).Methods("GET")

	r.PathPrefix("/").Handler(http.StripPrefix("/", vueServe(http.Dir("./web/dist"))))

	addr := viper.GetString("app.addr")
	srv := &http.Server{
		Handler: r,
		Addr:    addr,
	}
	go func() {
		err = srv.ListenAndServe()
		if err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()
	log.Infof("bell started on %s", addr)

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-done
	log.Print("Server Stopped")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		log.Println("Closing")
		if cronService != nil {
			log.Printf("Stopping cron service")
			cronService.Stop()
		}

		cancel()
	}()

	err = srv.Shutdown(ctx)
	if err != nil {
		log.Fatalf("Server Shutdown Failed: %v", err)
	}
	log.Print("Server shutdown gracefully")
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		start := time.Now()
		next.ServeHTTP(response, request)
		log.WithFields(log.Fields{
			"IP":     getIPAddress(request),
			"Method": request.Method,
			"URI":    request.RequestURI,
			"Cost":   time.Since(start).String(),
		}).Info("Handler called")
	})
}

func getIPAddress(r *http.Request) string {
	// for _, h := range []string{"X-Forwarded-For", "X-Real-Ip"} {
	for _, h := range []string{"X-Forwarded-For"} {
		addresses := strings.Split(r.Header.Get(h), ",")
		for i := 0; i < len(addresses); i++ {
			ip := strings.TrimSpace(addresses[i])
			// header can contain spaces too, strip those out.
			realIP := net.ParseIP(ip)
			if !realIP.IsGlobalUnicast() {
				// bad address, go to next
				continue
			}
			return ip
		}
	}
	return "localhost"
}

// func writeJSON(w http.ResponseWriter, httpStatusCode int, obj interface{}) {
// 	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
// 	result, err := json.Marshal(obj)
// 	if err != nil {
// 		log.Printf("Could not marshal result: %v", err)
// 		w.WriteHeader(http.StatusInternalServerError)
// 		w.Write([]byte("Could not marshal result: " + err.Error()))
// 		return
// 	}
// 	w.WriteHeader(httpStatusCode)
// 	w.Write(result)
// }

// func writeJSONBlob(w http.ResponseWriter, httpStatusCode int, obj []byte) {
// 	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
// 	w.WriteHeader(httpStatusCode)
// 	w.Write(obj)
// }

// func writeJSONCache(w http.ResponseWriter, httpStatusCode int, obj []byte) {
// 	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
// 	w.WriteHeader(httpStatusCode)
// 	w.Write([]byte(obj))
// }

// func writeCSV(w http.ResponseWriter, httpStatusCode int, obj string) {
// 	w.Header().Set("Content-Type", "text/csv; charset=UTF-8")
// 	w.WriteHeader(httpStatusCode)
// 	w.Write([]byte(obj))
// }

// func writePDF(w http.ResponseWriter, httpStatusCode int, obj []byte) {
// 	w.Header().Set("Content-Type", "application/pdf")
// 	w.WriteHeader(httpStatusCode)
// 	w.Write(obj)
// }

// func writePNG(w http.ResponseWriter, httpStatusCode int, obj []byte) {
// 	w.Header().Set("Content-Type", "image/png")
// 	w.WriteHeader(httpStatusCode)
// 	w.Write(obj)
// }

// func writeWebp(w http.ResponseWriter, httpStatusCode int, obj []byte) {
// 	w.Header().Set("Content-Type", "image/webp")
// 	w.WriteHeader(httpStatusCode)
// 	w.Write(obj)
// }

// func writeJPEG(w http.ResponseWriter, httpStatusCode int, obj []byte) {
// 	w.Header().Set("Content-Type", "image/jpeg")
// 	w.WriteHeader(httpStatusCode)
// 	w.Write(obj)
// }

// func getBodyByteArray(r *http.Request) ([]byte, error) {
// 	body, err := io.ReadAll(io.LimitReader(r.Body, 1000000))
// 	if err != nil {
// 		log.Errorf("Could not parse body: %v", err)
// 		return nil, err
// 	}

// 	err = r.Body.Close()
// 	if err != nil {
// 		log.Errorf("Could not close body: %v", err)
// 		return nil, err
// 	}

// 	return body, nil
// }

func vueServe(fs http.FileSystem) http.Handler {
	log.Printf("creating file handler")
	fsh := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Opening: %s", path.Clean(r.URL.Path))
		_, err := fs.Open(path.Clean(r.URL.Path))
		if os.IsNotExist(err) {
			index, err := os.ReadFile("./web/dist/index.html")
			if err != nil {
				log.Errorf("Could not read ./web/dist/index.html: %v", err)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.Write(index)
			return
		}
		fsh.ServeHTTP(w, r)
	})
}
