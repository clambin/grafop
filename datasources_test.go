package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gosimple/slug"
	"github.com/grafana/grafana-openapi-client-go/client/datasources"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportDataSources(t *testing.T) {
	v := viper.New()
	v.Set("namespace", "monitoring")
	v.Set("grafana.operator.label.value", "local-grafana")
	v.Set("grafana.url", "http://grafana")
	v.Set("grafana.operator.label.name", "dashboards")
	v.Set("grafana.operator.label.value", "local-grafana")
	cfg := configurationFromViper(v)
	client := grafanaClient{
		Datasources: fakeDataSourceFetcher{
			dataSources: map[string]*models.DataSource{
				"prometheus": {ID: 0, Name: "prometheus", Type: "prometheus", URL: "http://prometheus"},
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, exportDatasources(&buf, &client, cfg, []string{"prometheus"}))

	gp := filepath.Join("testdata", slug.Make(t.Name())+".yaml")
	if *update {
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
