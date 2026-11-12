package main

import (
	"fmt"
	"net/url"

	"github.com/alecthomas/kong"
	"github.com/go-openapi/strfmt"
	goapi "github.com/grafana/grafana-openapi-client-go/client"
	"github.com/grafana/grafana-openapi-client-go/client/dashboards"
	"github.com/grafana/grafana-openapi-client-go/client/datasources"
	"github.com/grafana/grafana-openapi-client-go/client/search"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CLI struct {
	Grafana     grafanaConfig   `embed:"" prefix:"grafana-"`
	Config      kong.ConfigFlag `name:"config" help:"Path to configuration file" type:"path"`
	Namespace   string          `name:"namespace" default:"" help:"Namespace for k8s config maps (default: no namespace added)"`
	DataSources DataSourcesCmd  `cmd:"" name:"datasources" help:"export datasources"`
	Dashboards  DashboardsCmd   `cmd:"" help:"export dashboards"`
}

type grafanaConfig struct {
	URL      string          `name:"url"`
	Token    string          `name:"token" default:"" help:"Grafana API token (must have admin rights)"`
	Operator grafanaOperator `embed:"" prefix:"operator-"`
}

type grafanaOperator struct {
	Label grafanaOperatorLabel `embed:"" prefix:"label-"`
}
type grafanaOperatorLabel struct {
	Name  string `name:"name" default:"dashboards" help:"label name used to select the Grafana instance"`
	Value string `name:"value" default:"grafana" help:"label value used to select the Grafana instance"`
}

func (c grafanaConfig) client() (*grafanaClient, error) {
	target, err := url.Parse(c.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid Grafana url %q: %w", c.URL, err)
	}
	if target.Scheme == "" {
		return nil, fmt.Errorf("invalid grafana.url %q: invalid scheme %q", c.URL, target.Scheme)
	}
	cfg := goapi.TransportConfig{
		Host:     target.Host,
		BasePath: "/api",
		Schemes:  []string{target.Scheme},
		APIKey:   c.Token,
	}
	client := goapi.NewHTTPClientWithConfig(strfmt.Default, &cfg)
	return &grafanaClient{
		Search:      client.Search,
		Dashboards:  client.Dashboards,
		Datasources: client.Datasources,
	}, nil
}

func (c grafanaConfig) instanceSelector() (*metav1.LabelSelector, error) {
	if c.Operator.Label.Name == "" {
		return nil, fmt.Errorf("grafana.operator.label cannot be nil")
	}
	return &metav1.LabelSelector{MatchLabels: map[string]string{c.Operator.Label.Name: c.Operator.Label.Value}}, nil
}

type grafanaClient struct {
	Search      grafanaSearchClient
	Dashboards  grafanaDashboardClient
	Datasources grafanaDatasourcesClient
}

type grafanaSearchClient interface {
	Search(*search.SearchParams, ...search.ClientOption) (*search.SearchOK, error)
}

type grafanaDashboardClient interface {
	GetDashboardByUID(string, ...dashboards.ClientOption) (*dashboards.GetDashboardByUIDOK, error)
}

type grafanaDatasourcesClient interface {
	GetDataSourceByName(name string, opts ...datasources.ClientOption) (*datasources.GetDataSourceByNameOK, error)
}
