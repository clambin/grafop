package main

import (
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"time"

	"github.com/gosimple/slug"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/grafana/grafana-operator/v5/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// DataSourcesCmd is a Kong command that exports Grafana datasources.
type DataSourcesCmd struct {
	Args []string `arg:"" optional:"" name:"args" help:"Additional arguments"`
}

func (d DataSourcesCmd) Run(cli *CLI) error {
	client, err := cli.Grafana.client()
	if err != nil {
		return fmt.Errorf("failed to get grafana client: %w", err)
	}
	return d.export(os.Stdout, client, cli)
}

func (d DataSourcesCmd) export(w io.Writer, client *grafanaClient, cli *CLI) error {
	for datasource, err := range grafanaDataSources(client, d.Args) {
		if err != nil {
			return fmt.Errorf("failed to get datasources: %w", err)
		}
		if len(datasource.SecureJSONFields) > 0 {
			_, _ = fmt.Fprintf(os.Stderr, "datasource %q uses secure JSON fields and requires manual changes. See https://grafana.github.io/grafana-operator/docs/examples/datasource/\n", datasource.Name)
		}
		ds, err := d.operatorDatasource(datasource, cli)
		if err != nil {
			return fmt.Errorf("failed to get operator datasource: %w", err)
		}
		body, err := yaml.Marshal(ds)
		if err != nil {
			return fmt.Errorf("failed to marshal operator datasource: %w", err)
		}
		_, _ = w.Write([]byte("---\n"))
		_, _ = w.Write(body)

	}
	return nil
}

// datasourceManifest is a stripped-down version of Grafana Operator Datasource custom resource.
// This allows us to marshal the datasource to YAML without including the Status section.
type datasourceManifest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              v1beta1.GrafanaDatasourceSpec `json:"spec"`
}

func (d DataSourcesCmd) operatorDatasource(datasource *models.DataSource, cli *CLI) (datasourceManifest, error) {
	var jsonData json.RawMessage
	if datasource.JSONData != nil {
		jsonData, _ = json.Marshal(datasource.JSONData)
	}
	selector, err := cli.Grafana.instanceSelector()
	if err != nil {
		return datasourceManifest{}, fmt.Errorf("failed to get instance selector: %w", err)
	}
	return datasourceManifest{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       "GrafanaDatasource",
		Name:       slug.Make(datasource.Name),
		Namespace:  cli.Namespace,
		Spec: v1beta1.GrafanaDatasourceSpec{
			GrafanaCommonSpec: v1beta1.GrafanaCommonSpec{
				ResyncPeriod:              metav1.Duration{Duration: 10 * time.Minute},
				AllowCrossNamespaceImport: true,
				InstanceSelector:          selector,
			},
			// isn't there a way to get *GrafanaDatasourceInternal directly?
			Datasource: &v1beta1.GrafanaDatasourceInternal{
				UID:            datasource.UID,
				Name:           datasource.Name,
				Type:           datasource.Type,
				URL:            datasource.URL,
				Access:         string(datasource.Access),
				Database:       datasource.Database,
				User:           datasource.User,
				IsDefault:      &datasource.IsDefault,
				BasicAuth:      &datasource.BasicAuth,
				BasicAuthUser:  datasource.BasicAuthUser,
				Editable:       new(false), // TODO: editable even if this is false.
				JSONData:       jsonData,
				SecureJSONData: nil, // unavailable from the grafana API.
			},
		},
	}, nil
}

// grafanaDataSources returns all datasources that match the names in args.
func grafanaDataSources(c *grafanaClient, args []string) iter.Seq2[*models.DataSource, error] {
	return func(yield func(*models.DataSource, error) bool) {
		for _, name := range args {
			ds, err := c.Datasources.GetDataSourceByName(name)
			if err != nil {
				yield(nil, fmt.Errorf("error getting datasource %q: %w", name, err))
				continue
			}
			if !yield(ds.GetPayload(), nil) {
				return
			}
		}
	}
}
