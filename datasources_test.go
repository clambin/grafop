package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/grafana-openapi-client-go/client/datasources"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportDataSources(t *testing.T) {
	client := grafanaClient{
		Datasources: fakeDataSourceFetcher{
			dataSources: map[string]*models.DataSource{
				"prometheus": {
					ID:               0,
					Name:             "prometheus",
					Type:             "prometheus",
					URL:              "http://prometheus",
					JSONData:         map[string]any{"foo": "bar"},
					SecureJSONFields: map[string]bool{"secure": true},
				},
			},
		},
	}
	cli := CLI{
		Namespace: "monitoring",
		Grafana: grafanaConfig{Operator: grafanaOperator{Label: grafanaOperatorLabel{
			Name:  "dashboards",
			Value: "local-grafana",
		}}},
		DataSources: DataSourcesCmd{
			Args: []string{"prometheus"},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, cli.DataSources.export(&buf, &client, &cli))

	gp := filepath.Join("testdata", strings.ToLower(t.Name())+".yaml")
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(gp), 0755))
		require.NoError(t, os.WriteFile(gp, buf.Bytes(), 0644))
	}
	golden, err := os.ReadFile(gp)
	require.NoError(t, err)
	assert.Equal(t, string(golden), buf.String())
}

var _ grafanaDatasourcesClient = &fakeDataSourceFetcher{}

type fakeDataSourceFetcher struct {
	//err         error
	dataSources map[string]*models.DataSource
}

func (f fakeDataSourceFetcher) GetDataSourceByName(name string, _ ...datasources.ClientOption) (*datasources.GetDataSourceByNameOK, error) {
	if ds, ok := f.dataSources[name]; ok {
		result := datasources.NewGetDataSourceByNameOK()
		result.Payload = ds
		return result, nil
	}
	return nil, errors.New("not found")
}
