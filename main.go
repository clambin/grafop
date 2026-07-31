package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"codeberg.org/clambin/go-common/charmer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	configFilename string
	rootCmd        = cobra.Command{
		Use:   "grafop",
		Short: "exports Grafana dashboards & datasources as grafana-operator custom resources",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			charmer.SetTextLogger(cmd, viper.GetBool("debug"))
		},
	}
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		charmer.GetLogger(&rootCmd).Error("failed to run", "err", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	initArgs()
}

var args = charmer.Arguments{
	"debug":                        {Default: false, Help: "Log debug messages"},
	"namespace":                    {Default: "", Help: "Namespace for k8s config maps (default: no namespace added)"},
	"tags":                         {Default: "", Help: "Dashboard tags (comma-separated; optional)"},
	"grafana.url":                  {Default: "http://localhost:3000", Help: "Grafana URL"},
	"grafana.token":                {Default: "", Help: "Grafana API token (must have admin rights)"},
	"grafana.operator.label.name":  {Default: "dashboards", Help: "label used to select the grafana instance"},
	"grafana.operator.label.value": {Default: "grafana", Help: "label value used to select the grafana instance"},
}

func initArgs() {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		rootCmd.Version = buildInfo.Main.Version
	}

	rootCmd.PersistentFlags().StringVarP(&configFilename, "config", "c", "", "Configuration file")
	_ = charmer.SetPersistentFlags(&rootCmd, viper.GetViper(), args)
}

func initConfig() {
	initViper(viper.GetViper())
}

func initViper(v *viper.Viper) {
	if configFilename != "" {
		v.SetConfigFile(configFilename)
	} else {
		v.AddConfigPath("/etc/grafop/")
		v.AddConfigPath("$HOME/.grafop")
		v.AddConfigPath(".")
		v.SetConfigName("config")
	}

	v.SetEnvPrefix("GRAFOP")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to read config file: %s\n", err.Error())
	}
}
