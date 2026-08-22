package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-openapi/strfmt"
	goapi "github.com/grafana/grafana-openapi-client-go/client"
	"github.com/grafana/grafana-openapi-client-go/client/dashboards"
	"github.com/grafana/grafana-openapi-client-go/client/datasources"
	"github.com/grafana/grafana-openapi-client-go/client/search"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type configuration struct {
	Grafana   grafanaConfiguration
	Namespace string
	Tags      []string
	Folders   bool
}

type grafanaConfiguration struct {
	URL      string
	Token    string
	Operator grafanaOperatorConfiguration
}

type grafanaOperatorConfiguration struct {
	Labels map[string]string
}

func configurationFromViper(v *viper.Viper) configuration {
	var labels map[string]string
	if name := v.GetString("grafana.operator.label.name"); name != "" {
		labels = map[string]string{
			name: v.GetString("grafana.operator.label.value"),
		}
	} else {
		labels = map[string]string{
			"dashboards": "grafana",
		}
	}
	var tags []string
	if tagsArg := v.GetString("tags"); tagsArg != "" {
		tags = strings.Split(tagsArg, ",")
	}
	return configuration{
		Grafana: grafanaConfiguration{
			URL:   v.GetString("grafana.url"),
			Token: v.GetString("grafana.token"),
			Operator: grafanaOperatorConfiguration{
				Labels: labels,
			},
		},
		Namespace: v.GetString("namespace"),
		Tags:      tags,
		Folders:   v.GetBool("folders"),
	}
}

func (c configuration) grafanaClient() (*grafanaClient, error) {
	target, err := url.Parse(c.Grafana.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid grafana.url %q: %w", c.Grafana.URL, err)
	}
	if target.Scheme == "" {
		return nil, fmt.Errorf("invalid grafana.url %q: invalid scheme %q", c.Grafana.URL, target.Scheme)
	}
	cfg := goapi.TransportConfig{
		Host:     target.Host,
		BasePath: "/api",
		Schemes:  []string{target.Scheme},
		APIKey:   c.Grafana.Token,
	}
	client := goapi.NewHTTPClientWithConfig(strfmt.Default, &cfg)
	return &grafanaClient{
		Search:      client.Search,
		Dashboards:  client.Dashboards,
		Datasources: client.Datasources,
	}, nil
}

func (c configuration) instanceSelector() *metav1.LabelSelector {
	if c.Grafana.Operator.Labels == nil {
		return nil
	}
	return &metav1.LabelSelector{MatchLabels: c.Grafana.Operator.Labels}
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
