package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"time"

	"codeberg.org/clambin/go-common/set"
	"github.com/gosimple/slug"
	"github.com/grafana/grafana-openapi-client-go/client/search"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/grafana/grafana-operator/v5/api/v1beta1"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml" // use sigs.k8s.io/yaml as it contains magic to marshal k8s definitions to YAML
)

var (
	dashboardsCmd = &cobra.Command{
		Use:   "dashboards [flags] [name [...]]",
		Short: "export Grafana dashboards",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := configurationFromViper(viper.GetViper())
			client, err := cfg.grafanaClient()
			if err != nil {
				return fmt.Errorf("grafana: %w", err)
			}
			return exportDashboards(os.Stdout, client, cfg, set.New(args...))
		},
	}
)

func init() {
	rootCmd.AddCommand(dashboardsCmd)
	dashboardsCmd.Flags().BoolP("folders", "f", false, "Export folder")
	_ = viper.BindPFlag("folders", dashboardsCmd.Flags().Lookup("folders"))
}

type exportedDashboard struct {
	Hit       *models.Hit
	Dashboard *models.DashboardFullWithMeta
}

func exportDashboards(
	w io.Writer,
	client *grafanaClient,
	cfg configuration,
	args set.Set[string],
) error {
	for dashboard, err := range grafanaDashboards(client, cfg.Folders, args) {
		if err != nil {
			return err
		}
		db, err := operatorDashboard(cfg, dashboard.Hit, dashboard.Dashboard)
		if err != nil {
			return fmt.Errorf("operator dashboard: %w", err)
		}
		body, err := yaml.Marshal(db)
		if err != nil {
			return fmt.Errorf("failed to marshal operator dashboard: %w", err)
		}
		_, _ = w.Write([]byte("---\n"))
		_, _ = w.Write(body)
	}
	return nil
}

// grafanaDashboards returns all Grafana dashboards that match args.
// If folders is false, it returns all dashboards whose title matches an element of args.
// Otherwise, it returns all dashboards in folders that matches an element of args.
func grafanaDashboards(c *grafanaClient, folders bool, args set.Set[string]) iter.Seq2[exportedDashboard, error] {
	return func(yield func(exportedDashboard, error) bool) {
		params := search.SearchParams{Type: new("dash-db")}
		var page int64
		for page = 1; ; page++ {
			params.Page = &page
			ok, err := c.Search.Search(&params)
			if err != nil {
				yield(exportedDashboard{}, fmt.Errorf("error getting dashboards: %w", err))
				return
			}
			hits := ok.GetPayload()
			if len(hits) == 0 {
				return
			}
			for _, entry := range hits {
				if len(args) > 0 {
					if (!folders && !args.Contains(entry.Title)) ||
						(folders && !args.Contains(entry.FolderTitle)) {
						continue
					}
				}
				db, err := c.Dashboards.GetDashboardByUID(entry.UID)
				if err != nil {
					yield(exportedDashboard{}, fmt.Errorf("error getting dashboard %q: %w", entry.Title, err))
					return
				}
				if !yield(exportedDashboard{Hit: entry, Dashboard: db.GetPayload()}, nil) {
					return
				}
			}
		}
	}
}

// dashboardManifest is a stripped-down version of Grafana Operator Dashboard custom resource.
// This allows us to marshal the dashboard to YAML without including the Status section.
type dashboardManifest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              v1beta1.GrafanaDashboardSpec `json:"spec"`
}

func operatorDashboard(cfg configuration, entry *models.Hit, dashboard *models.DashboardFullWithMeta) (dashboardManifest, error) {
	if err := tagDashboard(dashboard, cfg.Tags...); err != nil {
		return dashboardManifest{}, fmt.Errorf("failed to tag dashboard: %w", err)
	}

	var encodedDashboard bytes.Buffer
	jEnc := json.NewEncoder(&encodedDashboard)
	jEnc.SetIndent("", "  ")
	if err := jEnc.Encode(dashboard.Dashboard); err != nil {
		return dashboardManifest{}, fmt.Errorf("json: %w", err)
	}

	return dashboardManifest{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       "GrafanaDashboard",
		Name:       slug.Make(entry.Title),
		Namespace:  cfg.Namespace,
		Spec: v1beta1.GrafanaDashboardSpec{
			GrafanaCommonSpec: v1beta1.GrafanaCommonSpec{
				ResyncPeriod:              metav1.Duration{Duration: 10 * time.Minute},
				AllowCrossNamespaceImport: true,
				InstanceSelector:          cfg.instanceSelector(),
			},
			GrafanaContentSpec: v1beta1.GrafanaContentSpec{
				JSON: encodedDashboard.String(),
			},
			FolderTitle: entry.FolderTitle,
		},
	}, nil
}

// tagDashboard adds tags to the dashboard's model.
func tagDashboard(db *models.DashboardFullWithMeta, newTags ...string) error {
	jsonModel, ok := db.Dashboard.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected model type: %T; expected map[string]any", db.Dashboard)
	}
	tagsAny, ok := jsonModel["tags"]
	if !ok {
		tagsAny = any([]any{})
	}
	currentTags, _ := tagsAny.([]any)
	for _, newTag := range newTags {
		var found bool
		for _, currentTag := range currentTags {
			if currentTagAsString, ok := currentTag.(string); ok && currentTagAsString == newTag {
				found = true
				break
			}
		}
		if !found {
			currentTags = append(currentTags, newTag)
		}
	}
	jsonModel["tags"] = currentTags
	return nil
}
