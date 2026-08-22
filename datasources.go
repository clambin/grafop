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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

var (
	dataSourcesCmd = &cobra.Command{
		Use:   "datasources <name> [ <name> ...]",
		Short: "export Grafana data sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := configurationFromViper(viper.GetViper())
			client, err := cfg.grafanaClient()
			if err != nil {
				return fmt.Errorf("grafana: %w", err)
			}
			return exportDatasources(os.Stdout, client, cfg, args)
		},
	}
)

func init() {
	rootCmd.AddCommand(dataSourcesCmd)
}

func exportDatasources(
	w io.Writer,
	client *grafanaClient,
	cfg configuration,
	args []string,
) error {

	for datasource := range grafanaDataSources(client, args) {
		if len(datasource.SecureJSONFields) > 0 {
			_, _ = fmt.Fprintf(os.Stderr, "datasource %q uses secure JSON fields and requires manual changes. See https://grafana.github.io/grafana-operator/docs/datasources/\n", datasource.Name)
		}
		body, err := yaml.Marshal(operatorDatasource(cfg, datasource))
		if err != nil {
			return fmt.Errorf("failed to marshal operator datasource: %w", err)
		}
		_, _ = w.Write([]byte("---\n"))
		_, _ = w.Write(body)

	}
	return nil
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

// datasourceManifest is a stripped-down version of Grafana Operator Datasource custom resource.
// This allows us to marshal the datasource to YAML without including the Status section.
type datasourceManifest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              v1beta1.GrafanaDatasourceSpec `json:"spec"`
}

func operatorDatasource(cfg configuration, datasource *models.DataSource) datasourceManifest {
	var jsonData json.RawMessage
	if datasource.JSONData != nil {
		jsonData, _ = json.Marshal(datasource.JSONData)
	}
	return datasourceManifest{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       "GrafanaDatasource",
		Name:       slug.Make(datasource.Name),
		Namespace:  cfg.Namespace,
		Spec: v1beta1.GrafanaDatasourceSpec{
			GrafanaCommonSpec: v1beta1.GrafanaCommonSpec{
				ResyncPeriod:              metav1.Duration{Duration: 10 * time.Minute},
				AllowCrossNamespaceImport: true,
				InstanceSelector:          cfg.instanceSelector(),
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
				OrgID:          &datasource.OrgID,
				Editable:       new(false), // TODO: editable even if this is false.
				JSONData:       jsonData,
				SecureJSONData: nil, // unavailable from the grafana API.
			},
		},
	}
}
