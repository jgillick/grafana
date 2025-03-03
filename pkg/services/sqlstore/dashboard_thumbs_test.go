package sqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/models"
	dashver "github.com/grafana/grafana/pkg/services/dashboardversion"
	"github.com/grafana/grafana/pkg/util"
	"github.com/stretchr/testify/require"
)

var theme = models.ThemeDark
var kind = models.ThumbnailKindDefault

func TestIntegrationSqlStorage(t *testing.T) {
	var sqlStore *SQLStore
	var savedFolder *models.Dashboard

	setup := func() {
		sqlStore = InitTestDB(t)
		savedFolder = insertTestDashboard(t, sqlStore, "1 test dash folder", 1, 0, true, "prod", "webapp")
	}

	t.Run("Should insert dashboard in default state", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, dash.Version)
		thumb := getThumbnail(t, sqlStore, dash.Uid, dash.OrgId)

		require.Positive(t, thumb.Id)
		require.Equal(t, models.ThumbnailStateDefault, thumb.State)
		require.Equal(t, dash.Version, thumb.DashboardVersion)
	})

	t.Run("Should be able to update the thumbnail", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, dash.Version)
		thumb := getThumbnail(t, sqlStore, dash.Uid, dash.OrgId)

		insertedThumbnailId := thumb.Id
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, dash.Version+1)

		updatedThumb := getThumbnail(t, sqlStore, dash.Uid, dash.OrgId)
		require.Equal(t, insertedThumbnailId, updatedThumb.Id)
		require.Equal(t, dash.Version+1, updatedThumb.DashboardVersion)
	})

	t.Run("Should return empty array if all dashboards have thumbnails", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")

		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, dash.Version)

		cmd := models.FindDashboardsWithStaleThumbnailsCommand{
			Kind:  kind,
			Theme: theme,
		}
		res, err := sqlStore.FindDashboardsWithStaleThumbnails(context.Background(), &cmd)
		require.NoError(t, err)
		require.Len(t, res, 0)
	})

	t.Run("Should return dashboards with thumbnails marked as stale", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, dash.Version)
		updateThumbnailState(t, sqlStore, dash.Uid, dash.OrgId, models.ThumbnailStateStale)

		cmd := models.FindDashboardsWithStaleThumbnailsCommand{
			Kind:  kind,
			Theme: theme,
		}
		res, err := sqlStore.FindDashboardsWithStaleThumbnails(context.Background(), &cmd)
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Equal(t, dash.Id, res[0].Id)
	})

	t.Run("Should not return dashboards with updated thumbnails that had been marked as stale", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, dash.Version)
		updateThumbnailState(t, sqlStore, dash.Uid, dash.OrgId, models.ThumbnailStateStale)
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, dash.Version)

		cmd := models.FindDashboardsWithStaleThumbnailsCommand{
			Kind:  kind,
			Theme: theme,
		}
		res, err := sqlStore.FindDashboardsWithStaleThumbnails(context.Background(), &cmd)
		require.NoError(t, err)
		require.Len(t, res, 0)
	})

	t.Run("Should find dashboards without thumbnails", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")

		cmd := models.FindDashboardsWithStaleThumbnailsCommand{
			Kind:  kind,
			Theme: theme,
		}
		res, err := sqlStore.FindDashboardsWithStaleThumbnails(context.Background(), &cmd)
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Equal(t, dash.Id, res[0].Id)
	})

	t.Run("Should find dashboards with outdated thumbnails", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, dash.Version)

		updateTestDashboard(t, sqlStore, dash, map[string]interface{}{
			"tags": "different-tag",
		})

		cmd := models.FindDashboardsWithStaleThumbnailsCommand{
			Kind:  kind,
			Theme: theme,
		}
		res, err := sqlStore.FindDashboardsWithStaleThumbnails(context.Background(), &cmd)
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Equal(t, dash.Id, res[0].Id)
	})

	t.Run("Should not return dashboards with locked thumbnails even if they are outdated", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, dash.Version)
		updateThumbnailState(t, sqlStore, dash.Uid, dash.OrgId, models.ThumbnailStateLocked)

		updateTestDashboard(t, sqlStore, dash, map[string]interface{}{
			"tags": "different-tag",
		})

		cmd := models.FindDashboardsWithStaleThumbnailsCommand{
			Kind:  kind,
			Theme: theme,
		}
		res, err := sqlStore.FindDashboardsWithStaleThumbnails(context.Background(), &cmd)
		require.NoError(t, err)
		require.Len(t, res, 0)
	})

	t.Run("Should not return dashboards with manually uploaded thumbnails by default", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, models.DashboardVersionForManualThumbnailUpload)

		updateTestDashboard(t, sqlStore, dash, map[string]interface{}{
			"tags": "different-tag",
		})

		cmd := models.FindDashboardsWithStaleThumbnailsCommand{
			Kind:  kind,
			Theme: theme,
		}
		res, err := sqlStore.FindDashboardsWithStaleThumbnails(context.Background(), &cmd)
		require.NoError(t, err)
		require.Len(t, res, 0)
	})

	t.Run("Should return dashboards with manually uploaded thumbnails if requested", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, models.DashboardVersionForManualThumbnailUpload)

		updateTestDashboard(t, sqlStore, dash, map[string]interface{}{
			"tags": "different-tag",
		})

		cmd := models.FindDashboardsWithStaleThumbnailsCommand{
			Kind:                              kind,
			Theme:                             theme,
			IncludeManuallyUploadedThumbnails: true,
		}
		res, err := sqlStore.FindDashboardsWithStaleThumbnails(context.Background(), &cmd)
		require.NoError(t, err)
		require.Len(t, res, 1)
		require.Equal(t, dash.Id, res[0].Id)
	})

	t.Run("Should count all dashboard thumbnails", func(t *testing.T) {
		setup()
		dash := insertTestDashboard(t, sqlStore, "test dash 23", 1, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash.Uid, dash.OrgId, 1)
		dash2 := insertTestDashboard(t, sqlStore, "test dash 23", 2, savedFolder.Id, false, "prod", "webapp")
		upsertTestDashboardThumbnail(t, sqlStore, dash2.Uid, dash2.OrgId, 1)

		updateTestDashboard(t, sqlStore, dash, map[string]interface{}{
			"tags": "different-tag",
		})

		cmd := models.FindDashboardThumbnailCountCommand{}
		res, err := sqlStore.FindThumbnailCount(context.Background(), &cmd)
		require.NoError(t, err)
		require.Equal(t, res, int64(2))
	})
}

func getThumbnail(t *testing.T, sqlStore *SQLStore, dashboardUID string, orgId int64) *models.DashboardThumbnail {
	t.Helper()
	cmd := models.GetDashboardThumbnailCommand{
		DashboardThumbnailMeta: models.DashboardThumbnailMeta{
			DashboardUID: dashboardUID,
			OrgId:        orgId,
			PanelID:      0,
			Kind:         kind,
			Theme:        theme,
		},
	}

	thumb, err := sqlStore.GetThumbnail(context.Background(), &cmd)
	require.NoError(t, err)
	return thumb
}

func upsertTestDashboardThumbnail(t *testing.T, sqlStore *SQLStore, dashboardUID string, orgId int64, dashboardVersion int) *models.DashboardThumbnail {
	t.Helper()
	cmd := models.SaveDashboardThumbnailCommand{
		DashboardThumbnailMeta: models.DashboardThumbnailMeta{
			DashboardUID: dashboardUID,
			OrgId:        orgId,
			PanelID:      0,
			Kind:         kind,
			Theme:        theme,
		},
		DashboardVersion: dashboardVersion,
		Image:            make([]byte, 0),
		MimeType:         "image/png",
	}
	dash, err := sqlStore.SaveThumbnail(context.Background(), &cmd)
	require.NoError(t, err)
	require.NotNil(t, dash)

	return dash
}

func updateThumbnailState(t *testing.T, sqlStore *SQLStore, dashboardUID string, orgId int64, state models.ThumbnailState) {
	t.Helper()
	cmd := models.UpdateThumbnailStateCommand{
		DashboardThumbnailMeta: models.DashboardThumbnailMeta{
			DashboardUID: dashboardUID,
			OrgId:        orgId,
			PanelID:      0,
			Kind:         kind,
			Theme:        theme,
		},
		State: state,
	}
	err := sqlStore.UpdateThumbnailState(context.Background(), &cmd)
	require.NoError(t, err)
}

func updateTestDashboard(t *testing.T, sqlStore *SQLStore, dashboard *models.Dashboard, data map[string]interface{}) {
	t.Helper()

	data["id"] = dashboard.Id

	parentVersion := dashboard.Version

	cmd := models.SaveDashboardCommand{
		OrgId:     dashboard.OrgId,
		Overwrite: true,
		Dashboard: simplejson.NewFromAny(data),
	}
	var dash *models.Dashboard
	err := sqlStore.WithDbSession(context.Background(), func(sess *DBSession) error {
		var existing models.Dashboard
		dash = cmd.GetDashboardModel()
		dashWithIdExists, err := sess.Where("id=? AND org_id=?", dash.Id, dash.OrgId).Get(&existing)
		require.NoError(t, err)
		require.True(t, dashWithIdExists)

		if dash.Version != existing.Version {
			dash.SetVersion(existing.Version)
			dash.Version = existing.Version
		}

		dash.SetVersion(dash.Version + 1)
		dash.Created = time.Now()
		dash.Updated = time.Now()
		dash.Id = dashboard.Id
		dash.Uid = util.GenerateShortUID()

		_, err = sess.MustCols("folder_id").ID(dash.Id).Update(dash)
		return err
	})

	require.Nil(t, err)

	err = sqlStore.WithDbSession(context.Background(), func(sess *DBSession) error {
		dashVersion := &dashver.DashboardVersion{
			DashboardID:   dash.Id,
			ParentVersion: parentVersion,
			RestoredFrom:  cmd.RestoredFrom,
			Version:       dash.Version,
			Created:       time.Now(),
			CreatedBy:     dash.UpdatedBy,
			Message:       cmd.Message,
			Data:          dash.Data,
		}

		if affectedRows, err := sess.Insert(dashVersion); err != nil {
			return err
		} else if affectedRows == 0 {
			return models.ErrDashboardNotFound
		}

		return nil
	})

	require.NoError(t, err)
}
