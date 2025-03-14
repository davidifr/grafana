package sqlstore

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/dashboards"
	dashver "github.com/grafana/grafana/pkg/services/dashboardversion"
	"github.com/grafana/grafana/pkg/services/org"
	"github.com/grafana/grafana/pkg/services/user"
	"github.com/grafana/grafana/pkg/util"
)

func TestIntegrationSQLBuilder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	t.Run("WriteDashboardPermissionFilter", func(t *testing.T) {
		t.Run("user ACL", func(t *testing.T) {
			test(t,
				DashboardProps{},
				&DashboardPermission{User: true, Permission: models.PERMISSION_VIEW},
				Search{UserFromACL: true, RequiredPermission: models.PERMISSION_VIEW},
				shouldFind,
			)

			test(t,
				DashboardProps{},
				&DashboardPermission{User: true, Permission: models.PERMISSION_VIEW},
				Search{UserFromACL: true, RequiredPermission: models.PERMISSION_EDIT},
				shouldNotFind,
			)

			test(t,
				DashboardProps{},
				&DashboardPermission{User: true, Permission: models.PERMISSION_EDIT},
				Search{UserFromACL: true, RequiredPermission: models.PERMISSION_EDIT},
				shouldFind,
			)

			test(t,
				DashboardProps{},
				&DashboardPermission{User: true, Permission: models.PERMISSION_VIEW},
				Search{RequiredPermission: models.PERMISSION_VIEW},
				shouldNotFind,
			)
		})

		t.Run("role ACL", func(t *testing.T) {
			test(t,
				DashboardProps{},
				&DashboardPermission{Role: org.RoleViewer, Permission: models.PERMISSION_VIEW},
				Search{UsersOrgRole: org.RoleViewer, RequiredPermission: models.PERMISSION_VIEW},
				shouldFind,
			)

			test(t,
				DashboardProps{},
				&DashboardPermission{Role: org.RoleViewer, Permission: models.PERMISSION_VIEW},
				Search{UsersOrgRole: org.RoleViewer, RequiredPermission: models.PERMISSION_EDIT},
				shouldNotFind,
			)

			test(t,
				DashboardProps{},
				&DashboardPermission{Role: org.RoleEditor, Permission: models.PERMISSION_VIEW},
				Search{UsersOrgRole: org.RoleViewer, RequiredPermission: models.PERMISSION_VIEW},
				shouldNotFind,
			)

			test(t,
				DashboardProps{},
				&DashboardPermission{Role: org.RoleEditor, Permission: models.PERMISSION_VIEW},
				Search{UsersOrgRole: org.RoleViewer, RequiredPermission: models.PERMISSION_VIEW},
				shouldNotFind,
			)
		})

		t.Run("team ACL", func(t *testing.T) {
			test(t,
				DashboardProps{},
				&DashboardPermission{Team: true, Permission: models.PERMISSION_VIEW},
				Search{UserFromACL: true, RequiredPermission: models.PERMISSION_VIEW},
				shouldFind,
			)

			test(t,
				DashboardProps{},
				&DashboardPermission{Team: true, Permission: models.PERMISSION_VIEW},
				Search{UserFromACL: true, RequiredPermission: models.PERMISSION_EDIT},
				shouldNotFind,
			)

			test(t,
				DashboardProps{},
				&DashboardPermission{Team: true, Permission: models.PERMISSION_EDIT},
				Search{UserFromACL: true, RequiredPermission: models.PERMISSION_EDIT},
				shouldFind,
			)

			test(t,
				DashboardProps{},
				&DashboardPermission{Team: true, Permission: models.PERMISSION_EDIT},
				Search{UserFromACL: false, RequiredPermission: models.PERMISSION_EDIT},
				shouldNotFind,
			)
		})

		t.Run("defaults for user ACL", func(t *testing.T) {
			test(t,
				DashboardProps{},
				nil,
				Search{OrgId: -1, UsersOrgRole: org.RoleViewer, RequiredPermission: models.PERMISSION_VIEW},
				shouldNotFind,
			)

			test(t,
				DashboardProps{OrgId: -1},
				nil,
				Search{OrgId: -1, UsersOrgRole: org.RoleViewer, RequiredPermission: models.PERMISSION_VIEW},
				shouldFind,
			)

			test(t,
				DashboardProps{OrgId: -1},
				nil,
				Search{OrgId: -1, UsersOrgRole: org.RoleEditor, RequiredPermission: models.PERMISSION_EDIT},
				shouldFind,
			)

			test(t,
				DashboardProps{OrgId: -1},
				nil,
				Search{OrgId: -1, UsersOrgRole: org.RoleViewer, RequiredPermission: models.PERMISSION_EDIT},
				shouldNotFind,
			)
		})
	})
}

const shouldFind = true
const shouldNotFind = false

type DashboardProps struct {
	OrgId int64
}

type DashboardPermission struct {
	User       bool
	Team       bool
	Role       org.RoleType
	Permission models.PermissionType
}

type Search struct {
	UsersOrgRole       org.RoleType
	UserFromACL        bool
	RequiredPermission models.PermissionType
	OrgId              int64
}

type dashboardResponse struct {
	Id int64
}

func test(t *testing.T, dashboardProps DashboardProps, dashboardPermission *DashboardPermission, search Search, shouldFind bool) {
	t.Helper()

	t.Run("", func(t *testing.T) {
		// Will also cleanup the db
		sqlStore := InitTestDB(t)

		dashboard := createDummyDashboard(t, sqlStore, dashboardProps)

		var aclUserID int64
		if dashboardPermission != nil {
			aclUserID = createDummyACL(t, sqlStore, dashboardPermission, search, dashboard.Id)
			t.Logf("Created ACL with user ID %d\n", aclUserID)
		}
		dashboards := getDashboards(t, sqlStore, search, aclUserID)

		if shouldFind {
			require.Len(t, dashboards, 1, "Should return one dashboard")
			assert.Equal(t, dashboard.Id, dashboards[0].Id, "Should return created dashboard")
		} else {
			assert.Empty(t, dashboards, "Should not return any dashboard")
		}
	})
}

func createDummyUser(t *testing.T, sqlStore *SQLStore) *user.User {
	t.Helper()

	uid := strconv.Itoa(rand.Intn(9999999))
	usr := &user.User{
		Email:         uid + "@example.com",
		Login:         uid,
		Name:          uid,
		Company:       "",
		Password:      uid,
		EmailVerified: true,
		IsAdmin:       false,
		Created:       time.Now(),
		Updated:       time.Now(),
	}

	var id int64
	err := sqlStore.WithDbSession(context.Background(), func(sess *DBSession) error {
		sess.UseBool("is_admin")
		var err error
		id, err = sess.Insert(usr)
		return err
	})
	require.NoError(t, err)
	usr.ID = id
	return usr
}

func createDummyDashboard(t *testing.T, sqlStore *SQLStore, dashboardProps DashboardProps) *models.Dashboard {
	t.Helper()

	json, err := simplejson.NewJson([]byte(`{"schemaVersion":17,"title":"gdev dashboards","uid":"","version":1}`))
	require.NoError(t, err)

	saveDashboardCmd := models.SaveDashboardCommand{
		Dashboard:    json,
		UserId:       0,
		Overwrite:    false,
		Message:      "",
		RestoredFrom: 0,
		PluginId:     "",
		FolderId:     0,
		IsFolder:     false,
		UpdatedAt:    time.Time{},
	}
	if dashboardProps.OrgId != 0 {
		saveDashboardCmd.OrgId = dashboardProps.OrgId
	} else {
		saveDashboardCmd.OrgId = 1
	}

	dash := insertTestDashboard(t, sqlStore, "", saveDashboardCmd.OrgId, 0, false, nil)
	require.NoError(t, err)

	t.Logf("Created dashboard with ID %d and org ID %d\n", dash.Id, dash.OrgId)
	return dash
}

func createDummyACL(t *testing.T, sqlStore *SQLStore, dashboardPermission *DashboardPermission, search Search, dashboardID int64) int64 {
	t.Helper()

	acl := &models.DashboardACL{
		OrgID:       1,
		Created:     time.Now(),
		Updated:     time.Now(),
		Permission:  dashboardPermission.Permission,
		DashboardID: dashboardID,
	}

	var user *user.User
	if dashboardPermission.User {
		t.Logf("Creating user")
		user = createDummyUser(t, sqlStore)

		acl.UserID = user.ID
	}

	if dashboardPermission.Team {
		// TODO: Restore/refactor sqlBuilder tests after user, org and team services are split
		t.Skip("Creating team: skip, team service is moved")
	}

	if len(string(dashboardPermission.Role)) > 0 {
		acl.Role = &dashboardPermission.Role
	}

	err := updateDashboardACL(t, sqlStore, dashboardID, acl)
	require.NoError(t, err)
	if user != nil {
		return user.ID
	}
	return 0
}

func getDashboards(t *testing.T, sqlStore *SQLStore, search Search, aclUserID int64) []*dashboardResponse {
	t.Helper()

	old := sqlStore.Cfg.RBACEnabled
	sqlStore.Cfg.RBACEnabled = false
	defer func() {
		sqlStore.Cfg.RBACEnabled = old
	}()

	builder := NewSqlBuilder(sqlStore.Cfg)
	signedInUser := &user.SignedInUser{
		UserID: 9999999999,
	}

	if search.OrgId == 0 {
		signedInUser.OrgID = 1
	} else {
		signedInUser.OrgID = search.OrgId
	}

	if len(string(search.UsersOrgRole)) > 0 {
		signedInUser.OrgRole = search.UsersOrgRole
	} else {
		signedInUser.OrgRole = org.RoleViewer
	}
	if search.UserFromACL {
		signedInUser.UserID = aclUserID
	}

	var res []*dashboardResponse
	builder.Write("SELECT * FROM dashboard WHERE true")
	builder.WriteDashboardPermissionFilter(signedInUser, search.RequiredPermission)
	t.Logf("Searching for dashboards, SQL: %q\n", builder.GetSQLString())
	err := sqlStore.engine.SQL(builder.GetSQLString(), builder.params...).Find(&res)
	require.NoError(t, err)
	return res
}

// TODO: Use FakeDashboardStore when org has its own service
func insertTestDashboard(t *testing.T, sqlStore *SQLStore, title string, orgId int64,
	folderId int64, isFolder bool, tags ...interface{}) *models.Dashboard {
	t.Helper()
	cmd := models.SaveDashboardCommand{
		OrgId:    orgId,
		FolderId: folderId,
		IsFolder: isFolder,
		Dashboard: simplejson.NewFromAny(map[string]interface{}{
			"id":    nil,
			"title": title,
			"tags":  tags,
		}),
	}

	var dash *models.Dashboard
	err := sqlStore.WithDbSession(context.Background(), func(sess *DBSession) error {
		dash = cmd.GetDashboardModel()
		dash.SetVersion(1)
		dash.Created = time.Now()
		dash.Updated = time.Now()
		dash.Uid = util.GenerateShortUID()
		_, err := sess.Insert(dash)
		return err
	})

	require.NoError(t, err)
	require.NotNil(t, dash)
	dash.Data.Set("id", dash.Id)
	dash.Data.Set("uid", dash.Uid)

	err = sqlStore.WithDbSession(context.Background(), func(sess *DBSession) error {
		dashVersion := &dashver.DashboardVersion{
			DashboardID:   dash.Id,
			ParentVersion: dash.Version,
			RestoredFrom:  cmd.RestoredFrom,
			Version:       dash.Version,
			Created:       time.Now(),
			CreatedBy:     dash.UpdatedBy,
			Message:       cmd.Message,
			Data:          dash.Data,
		}
		require.NoError(t, err)

		if affectedRows, err := sess.Insert(dashVersion); err != nil {
			return err
		} else if affectedRows == 0 {
			return dashboards.ErrDashboardNotFound
		}

		return nil
	})
	require.NoError(t, err)

	return dash
}

// TODO: Use FakeDashboardStore when org has its own service
func updateDashboardACL(t *testing.T, sqlStore *SQLStore, dashboardID int64, items ...*models.DashboardACL) error {
	t.Helper()

	err := sqlStore.WithDbSession(context.Background(), func(sess *DBSession) error {
		_, err := sess.Exec("DELETE FROM dashboard_acl WHERE dashboard_id=?", dashboardID)
		if err != nil {
			return fmt.Errorf("deleting from dashboard_acl failed: %w", err)
		}

		for _, item := range items {
			item.Created = time.Now()
			item.Updated = time.Now()
			if item.UserID == 0 && item.TeamID == 0 && (item.Role == nil || !item.Role.IsValid()) {
				return models.ErrDashboardACLInfoMissing
			}

			if item.DashboardID == 0 {
				return models.ErrDashboardPermissionDashboardEmpty
			}

			sess.Nullable("user_id", "team_id")
			if _, err := sess.Insert(item); err != nil {
				return err
			}
		}

		// Update dashboard HasACL flag
		dashboard := models.Dashboard{HasACL: true}
		_, err = sess.Cols("has_acl").Where("id=?", dashboardID).Update(&dashboard)
		return err
	})
	return err
}
