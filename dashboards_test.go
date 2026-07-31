package main

import (
	"bytes"
	"errors"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/clambin/go-common/set"
	"github.com/gosimple/slug"
	"github.com/grafana/grafana-openapi-client-go/client/dashboards"
	"github.com/grafana/grafana-openapi-client-go/client/search"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update golden files")

func TestExportDashboards(t *testing.T) {
	tests := []struct {
		name    string
		config  func() *viper.Viper
		args    []string
		limit   int64
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "unfiltered",
			config: func() *viper.Viper {
				v := viper.New()
				v.Set("grafana.url", "http://grafana")
				return v
			},
			wantErr: assert.NoError,
		},
		{
			name: "with namespace",
			config: func() *viper.Viper {
				v := viper.New()
				v.Set("grafana.url", "http://grafana")
				v.Set("namespace", "foo")
				return v
			},
			wantErr: assert.NoError,
		},
		{
			name: "tagged",
			config: func() *viper.Viper {
				v := viper.New()
				v.Set("grafana.url", "http://grafana")
				v.Set("tags", "grafop")
				return v
			},
			wantErr: assert.NoError,
		},
		{
			name: "filtered by name",
			config: func() *viper.Viper {
				v := viper.New()
				v.Set("grafana.url", "http://grafana")
				return v
			},
			args:    []string{"db 1"},
			wantErr: assert.NoError,
		},
		{
			name: "filtered by folder",
			config: func() *viper.Viper {
				v := viper.New()
				v.Set("grafana.url", "http://grafana")
				v.Set("folders", true)
				return v
			},
			args:    []string{"folder 1"},
			wantErr: assert.NoError,
		},
		{
			name: "override",
			config: func() *viper.Viper {
				v := viper.New()
				v.Set("grafana.url", "http://grafana")
				v.Set("namespace", "application")
				v.Set("grafana.operator.label.name", "dashboards")
				v.Set("grafana.operator.label.value", "local-grafana")
				v.Set("folders", "folder 1")
				return v
			},
			wantErr: assert.NoError,
		},
		{
			name: "paged",
			config: func() *viper.Viper {
				v := viper.New()
				v.Set("grafana.url", "http://grafana")
				return v
			},
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.DiscardHandler)
			//logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
			cfg := configurationFromViper(tt.config())
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
			require.NoError(t, exportDashboards(&buf, &client, cfg, set.New(tt.args...), logger))

			gp := filepath.Join("testdata", slug.Make(t.Name())+".yaml")
			if *update {
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
	}
	limit := f.limit
	if limit == 0 {
		limit = int64(1000)
	}
	if params.Limit != nil {
		limit = *params.Limit
	}
	start := int(page-1) * int(limit)
	if start > len(f.hitList) {
		return result, nil
	}
	end := max(start+int(page), len(f.hitList))
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
