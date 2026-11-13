package main

import (
	"bytes"
	"cmp"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/grafana-openapi-client-go/client/dashboards"
	"github.com/grafana/grafana-openapi-client-go/client/search"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update golden files")

func TestExportDashboards(t *testing.T) {
	tests := []struct {
		name    string
		cmd     DashboardsCmd
		cli     CLI
		limit   int64
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "unfiltered",
			cli: CLI{Grafana: grafanaConfig{
				URL:      "http://grafana",
				Operator: grafanaOperator{Label: grafanaOperatorLabel{Name: "dashboards", Value: "grafana"}},
			}},
		},
		{
			name: "paged",
			cli: CLI{Grafana: grafanaConfig{
				URL:      "http://grafana",
				Operator: grafanaOperator{Label: grafanaOperatorLabel{Name: "dashboards", Value: "grafana"}},
			}},
			limit: 1,
		},
		{
			name: "with namespace",
			cli: CLI{
				Namespace: "foo",
				Grafana: grafanaConfig{
					URL:      "http://grafana",
					Operator: grafanaOperator{Label: grafanaOperatorLabel{Name: "dashboards", Value: "grafana"}},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "tagged",
			cmd: DashboardsCmd{
				Tags: []string{"grafop"},
			},
			cli: CLI{
				Grafana: grafanaConfig{
					URL:      "http://grafana",
					Operator: grafanaOperator{Label: grafanaOperatorLabel{Name: "dashboards", Value: "grafana"}},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "filtered by name",
			cmd: DashboardsCmd{
				Args: []string{"db 1"},
			},
			cli: CLI{
				Grafana: grafanaConfig{
					URL:      "http://grafana",
					Operator: grafanaOperator{Label: grafanaOperatorLabel{Name: "dashboards", Value: "grafana"}},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "filtered by folder",
			cmd: DashboardsCmd{
				Folders: true,
				Args:    []string{"folder 1"},
			},
			cli: CLI{
				Grafana: grafanaConfig{
					URL:      "http://grafana",
					Operator: grafanaOperator{Label: grafanaOperatorLabel{Name: "dashboards", Value: "grafana"}},
				},
			},
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := grafanaClient{
				Search: fakeSearcher{
					limit: tt.limit,
					hitList: models.HitList{
						{Title: "db 1", FolderTitle: "folder 1", Type: "dash-db", UID: "1"},
						{Title: "db 2", FolderTitle: "folder 2", Type: "dash-db", UID: "2"},
					},
				},
				Dashboards: fakeDashboardFetcher{dashboards: map[string]any{
					"1": map[string]any{"foo": "bar", "tags": []any{}},
					"2": map[string]any{"foo": "bar", "tags": []any{}},
				}},
			}

			var buf bytes.Buffer
			require.NoError(t, tt.cmd.export(&buf, &client, &tt.cli))

			gp := filepath.Join("testdata", strings.ToLower(t.Name())+".yaml")
			if *update {
				require.NoError(t, os.MkdirAll(filepath.Dir(gp), 0755))
				require.NoError(t, os.WriteFile(gp, buf.Bytes(), 0644))
			}
			golden, err := os.ReadFile(gp)
			require.NoError(t, err)
			assert.Equal(t, string(golden), buf.String())
		})

	}
}

func Test_tagDashboard(t *testing.T) {
	tests := []struct {
		name    string
		db      models.DashboardFullWithMeta
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "valid: no tags",
			db: models.DashboardFullWithMeta{Dashboard: map[string]any{
				"tags": []any{},
			}},
			wantErr: assert.NoError,
		},
		{
			name: "valid: tags",
			db: models.DashboardFullWithMeta{Dashboard: map[string]any{
				"tags": []any{"foo", "bar"},
			}},
			wantErr: assert.NoError,
		},
		{
			name: "valid: tag already exists",
			db: models.DashboardFullWithMeta{Dashboard: map[string]any{
				"tags": []any{"foo", "bar", "grafop"},
			}},
			wantErr: assert.NoError,
		},
		{
			name:    "valid: tags not present",
			db:      models.DashboardFullWithMeta{Dashboard: map[string]any{}},
			wantErr: assert.NoError,
		},
		{
			name: "invalid: tags invalid type",
			db: models.DashboardFullWithMeta{Dashboard: map[string]any{
				"tags": "foo",
			}},
			wantErr: assert.NoError,
		},
		{
			name:    "invalid: model invalid type",
			db:      models.DashboardFullWithMeta{Dashboard: "124"},
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tagDashboard(&tt.db, "grafop")
			tt.wantErr(t, err)

			if err == nil {
				assert.Contains(t, tt.db.Dashboard.(map[string]any)["tags"], "grafop")
			}
		})
	}
}

var _ grafanaSearchClient = fakeSearcher{}

type fakeSearcher struct {
	hitList models.HitList
	limit   int64
	err     error
}

func (f fakeSearcher) Search(params *search.SearchParams, _ ...search.ClientOption) (*search.SearchOK, error) {
	var result *search.SearchOK
	if f.err != nil {
		return nil, f.err
	}
	result = search.NewSearchOK()
	var page int64
	if params.Page != nil {
		page = *params.Page
	} else {
		page = 1
	}
	limit := cmp.Or(f.limit, 1000)
	if params.Limit != nil {
		limit = *params.Limit
	}
	start := int(page-1) * int(limit)
	if start >= len(f.hitList) {
		return result, nil
	}
	end := min(start+int(page), len(f.hitList))
	result.Payload = f.hitList[start:end]
	return result, f.err
}

var _ grafanaDashboardClient = fakeDashboardFetcher{}

type fakeDashboardFetcher struct {
	err        error
	dashboards map[string]any
}

func (f fakeDashboardFetcher) GetDashboardByUID(dashboardUID string, _ ...dashboards.ClientOption) (*dashboards.GetDashboardByUIDOK, error) {
	if f.err != nil {
		return nil, f.err
	}
	db, ok := f.dashboards[dashboardUID]
	if !ok {
		return nil, errors.New("dashboard not found")
	}
	result := dashboards.NewGetDashboardByUIDOK()
	result.Payload = &models.DashboardFullWithMeta{
		Dashboard: db,
	}
	return result, nil
}
