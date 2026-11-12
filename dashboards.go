package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"sync/atomic"
	"time"

	"codeberg.org/clambin/go-common/set"
	"github.com/gosimple/slug"
	"github.com/grafana/grafana-openapi-client-go/client/search"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/grafana/grafana-operator/v5/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml" // use sigs.k8s.io/yaml as it contains magic to marshal k8s definitions to YAML
)

// DashboardsCmd is a Kong command that exports Grafana dashboards.
type DashboardsCmd struct {
	Tags    []string `name:"tags" help:"Dashboards tags to add"`
	Args    []string `arg:"" optional:"" help:"Additional arguments"`
	Folders bool     `name:"folders" help:"Export folders"`
}

func (d DashboardsCmd) Run(cli *CLI) error {
	client, err := cli.Grafana.client()
	if err == nil {
		err = d.export(os.Stdout, client, cli)
	}
	return err
}

// export iterates over Grafana dashboards matching the provided names and exports them to YAML.
func (d DashboardsCmd) export(w io.Writer, client *grafanaClient, cli *CLI) error {
	// iterate through all matching dashboards
	i := dashboardIterator{
		folders: d.Folders,
		args:    set.New(d.Args...),
		client:  client,
	}
	for dashboard := range i.All() {
		// create an operator dashboard manifest
		db, err := d.operatorDashboard(cli, dashboard)
		if err != nil {
			return fmt.Errorf("operator dashboard: %w", err)
		}
		// marshal the dashboard to YAML
		body, err := yaml.Marshal(db)
		if err != nil {
			return fmt.Errorf("failed to marshal operator dashboard: %w", err)
		}
		// write the YAML to the writer
		_, _ = w.Write([]byte("---\n"))
		_, _ = w.Write(body)
	}
	return i.Err()
}

// dashboardManifest is a stripped-down version of Grafana Operator Dashboard custom resource.
// This allows us to marshal the dashboard to YAML without including the Status section.
type dashboardManifest struct {
	Spec              v1beta1.GrafanaDashboardSpec `json:"spec"`
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
}

// operatorDashboard converts a Grafana dashboard to a Grafana Operator Dashboard custom resource.
func (d DashboardsCmd) operatorDashboard(cli *CLI, entry exportedDashboard) (dashboardManifest, error) {
	if err := tagDashboard(entry.Dashboard, d.Tags...); err != nil {
		return dashboardManifest{}, fmt.Errorf("failed to tag dashboard: %w", err)
	}

	var encodedDashboard bytes.Buffer
	jEnc := json.NewEncoder(&encodedDashboard)
	jEnc.SetIndent("", "  ")
	if err := jEnc.Encode(entry.Dashboard.Dashboard); err != nil {
		return dashboardManifest{}, fmt.Errorf("json: %w", err)
	}

	selector, err := cli.Grafana.instanceSelector()
	if err != nil {
		return dashboardManifest{}, fmt.Errorf("failed to get instance selector: %w", err)
	}

	return dashboardManifest{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       "GrafanaDashboard",
		Name:       slug.Make(entry.Hit.Title),
		Namespace:  cli.Namespace,
		Spec: v1beta1.GrafanaDashboardSpec{
			GrafanaCommonSpec: v1beta1.GrafanaCommonSpec{
				ResyncPeriod:              metav1.Duration{Duration: 10 * time.Minute},
				AllowCrossNamespaceImport: true,
				InstanceSelector:          selector,
			},
			GrafanaContentSpec: v1beta1.GrafanaContentSpec{
				JSON: encodedDashboard.String(),
			},
			FolderTitle: entry.Hit.FolderTitle,
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

type exportedDashboard struct {
	Hit       *models.Hit
	Dashboard *models.DashboardFullWithMeta
}

// dashboardIterator is an iterator that returns all Grafana dashboards that match args.
// If folders is false, it returns all dashboards whose title matches an element of args.
// Otherwise, it returns all dashboards in folders that matches an element of args.
type dashboardIterator struct {
	args    set.Set[string]
	client  *grafanaClient
	err     atomic.Value
	folders bool
}

func (i *dashboardIterator) Err() error {
	err, _ := i.err.Load().(error)
	return err
}

func (i *dashboardIterator) All() iter.Seq[exportedDashboard] {
	return func(yield func(exportedDashboard) bool) {
		params := search.SearchParams{Type: new("dash-db")}
		var page int64
		// searching for dashboards returns pages of results
		// Grafana starts page numbering at 1
		for page = 1; ; page++ {
			// get the next page of results
			params.Page = &page
			ok, err := i.client.Search.Search(&params)
			if err != nil {
				i.err.Store(fmt.Errorf("error getting dashboards: %w", err))
				return
			}
			// get all dashboards
			hits := ok.GetPayload()
			if len(hits) == 0 {
				return
			}
			for _, entry := range hits {
				// skip dashboards that don't match the filter
				if !i.match(entry) {
					continue
				}
				// search doesn't return the actual dashboard, so we need to get it using the entry's UID
				db, err := i.client.Dashboards.GetDashboardByUID(entry.UID)
				if err != nil {
					i.err.Store(fmt.Errorf("error getting dashboard %q: %w", entry.Title, err))
					return
				}
				// yield the dashboard
				if !yield(exportedDashboard{Hit: entry, Dashboard: db.GetPayload()}) {
					return
				}
			}
		}
	}
}

func (i *dashboardIterator) match(entry *models.Hit) bool {
	switch {
	case len(i.args) == 0:
		return true
	case i.folders:
		return i.args.Contains(entry.FolderTitle)
	case !i.folders:
		return i.args.Contains(entry.Title)
	}
	return false
}
