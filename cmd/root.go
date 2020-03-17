package cmd

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	logrus_stack "github.com/Gurpartap/logrus-stack"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/ovh/catalyst/catalyser"
	"github.com/ovh/catalyst/core"
	"github.com/ovh/catalyst/middlewares"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// Catalyst init - define command line arguments.
func init() {
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file to use")

	RootCmd.Flags().IntP("log-level", "v", 4, "Log level (from 1 to 5)")
	RootCmd.Flags().Bool("dryrun", false, "Dry Run mode")
	RootCmd.Flags().StringP("listen", "l", "127.0.0.1:9100", "listen address")

	err := viper.BindPFlags(RootCmd.Flags())
	if err != nil {
		log.WithError(err).Fatalf("Couldn't parsed flags")
	}
}

// Load config - initialize defaults and read config.
func initConfig() {

	stackLevels := []log.Level{log.PanicLevel, log.FatalLevel, log.ErrorLevel}
	log.AddHook(logrus_stack.NewHook(stackLevels, stackLevels))

	// Default
	viper.SetDefault("warp_endpoint", "http://127.0.0.1:8080")
	viper.SetDefault("warp_endpoint_delete", "http://127.0.0.1:8080")
	viper.SetDefault("warp.connection.timeout", time.Second*300)
	viper.SetDefault("warp.connection.idle.max", 2000)
	viper.SetDefault("warp.connection.keep-alive.timeout", time.Second*30)
	viper.SetDefault("warp.connection.dial.timeout", 10*time.Second)
	viper.SetDefault("warp.connection.tls.timeout", 5*time.Second)
	viper.SetDefault("metrics.listen", "127.0.0.1:9105")
	viper.SetDefault("bannishment.duration", 3000)
	viper.SetDefault("graphite.listen", ":2003")
	viper.SetDefault("graphite.parse", true)

	hostname, err := os.Hostname()
	if err != nil {
		viper.Set("hostname", "unknowm")
	} else {
		viper.Set("hostname", hostname)
	}

	// Bind environment variables
	viper.SetEnvPrefix("catalyst")
	viper.AutomaticEnv()

	// Set config search path
	viper.AddConfigPath("/etc/catalyst/")
	viper.AddConfigPath("$HOME/.catalyst")
	viper.AddConfigPath(".")

	// Load config
	viper.SetConfigName("config")
	if err := viper.MergeInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Debug("No config file found")
		} else {
			log.Panicf("Fatal error in config file: %v \n", err)
		}
	}

	// Load user defined config
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		err := viper.ReadInConfig()
		if err != nil {
			log.Panicf("Fatal error in config file: %v \n", err)
		}
	}

	log.SetLevel(log.AllLevels[viper.GetInt("log-level")])
}

// RootCmd launch the aggregator agent.
var RootCmd = &cobra.Command{
	Use:   "catalyst",
	Short: "Catalyst multipass proxy",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info("Catalyst starting")
		router := echo.New()
		router.HideBanner = true

		router.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins:  []string{"*"},
			AllowMethods:  []string{"GET", "PUT", "POST", "DELETE"},
			AllowHeaders:  []string{"Origin", "Authorization", "Content-Type", "X-Warp10-Token"},
			ExposeHeaders: []string{},
		}))

		router.Use(middlewares.Logger())
		router.Use(middlewares.Bannishment(viper.GetDuration("bannishment.duration") * time.Millisecond))

		// Build catalysers
		openTSDB := core.NewHandler("opentsdb", []string{"POST"}, catalyser.OpenTSDB, nil)
		prometheus := core.NewHandler("prometheus", []string{"POST", "PUT"}, catalyser.Prometheus, nil)
		prometheusRemote := core.NewHandler("prometheus_remote_write", []string{"POST", "PUT"}, catalyser.HandleRemoteWrite, nil)
		influxdb := core.NewHandler("influxdb", []string{"POST"}, catalyser.InfluxDB, nil)
		graphite := core.NewHandler("graphite", []string{"POST"}, catalyser.GraphiteHTTP, nil)
		warp := core.NewHandler("warp", []string{"POST"}, catalyser.Warp, catalyser.WarpError)

		graphiteTCP := catalyser.NewGraphite(viper.GetString("graphite.listen"), viper.GetBool("graphite.parse"))
		go graphiteTCP.OpenTCPServer()

		// Support legacy
		router.Any("/opentsdb", openTSDB.Handle)
		router.Any("/prometheus", prometheus.Handle)
		router.Any("/warp", warp.Handle)
		router.Any("/influxdb", influxdb.Handle)
		router.Any("/graphite/api/v1/sink", graphite.Handle)

		router.Any("/opentsdb/*", openTSDB.Handle)
		router.Any("/prometheus/remote_write*", prometheusRemote.Handle)
		router.Any("/prometheus/*", prometheus.Handle)
		router.Any("/influxdb/write*", influxdb.Handle)
		router.Any("/influxdb/ping*", catalyser.HandlePing)
		router.Any("/warp/api/v0/update*", warp.Handle)
		router.Any("/warp/api/v0/delete*", middlewares.ReverseWithConfig(middlewares.ReverseConfig{
			URL:  viper.GetString("warp_endpoint_delete") + "/api/v0",
			Path: "/delete",
		}))
		router.Any("/warp/api/v0/*", middlewares.ReverseWithConfig(middlewares.ReverseConfig{
			URL: viper.GetString("warp_endpoint") + "/api/v0",
		}))

		metricsServer := echo.New()
		metricsServer.HideBanner = true

		metricsServer.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

		go func() {
			addr := viper.GetString("metrics.listen")
			log.Infof("Metrics listen %s", addr)
			if err := metricsServer.Start(addr); err != nil {
				if err == http.ErrServerClosed {
					log.Info("Gracefully close the metrics http server")
					return
				}
				log.WithError(err).Fatal("Could not start the metrics http server")
			}
		}()

		go func() {
			addr := viper.GetString("listen")
			log.Infof("Listen %s", addr)
			if err := router.Start(addr); err != nil {
				if err == http.ErrServerClosed {
					log.Info("Gracefully close the http server")
					return
				}
				log.WithError(err).Fatal("Could not start the http server")
			}
		}()

		// Wait for interrupt signal to gracefully shutdown the server
		quit := make(chan os.Signal, 2)

		signal.Notify(quit, syscall.SIGTERM)
		signal.Notify(quit, syscall.SIGINT)

		log.Info("Catalyst started")

		<-quit

		if err := router.Close(); err != nil {
			log.WithError(err).Error("Could not close the server")
		}

		if err := metricsServer.Close(); err != nil {
			log.WithError(err).Error("Could not close the metrics server")
		}
	},
}
