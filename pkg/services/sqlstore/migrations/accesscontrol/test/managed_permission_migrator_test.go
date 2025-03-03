package test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/services/accesscontrol"
	acmig "github.com/grafana/grafana/pkg/services/sqlstore/migrations/accesscontrol"
	"github.com/grafana/grafana/pkg/services/sqlstore/migrator"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestManagedPermissionsMigration(t *testing.T) {
	// Run initial migration to have a working DB
	x := setupTestDB(t)

	team1Scope := accesscontrol.Scope("teams", "id", "1")
	team2Scope := accesscontrol.Scope("teams", "id", "2")

	type teamMigrationTestCase struct {
		desc          string
		putRolePerms  map[int64]map[string][]rawPermission
		wantRolePerms map[int64]map[string][]rawPermission
	}
	testCases := []teamMigrationTestCase{
		{
			desc:          "empty perms",
			putRolePerms:  map[int64]map[string][]rawPermission{},
			wantRolePerms: map[int64]map[string][]rawPermission{},
		},
		{
			desc: "only unrelated perms",
			putRolePerms: map[int64]map[string][]rawPermission{
				1: {
					"managed:users:1:permissions": {{Action: "teams:read", Scope: team1Scope}},
				},
			},
			wantRolePerms: map[int64]map[string][]rawPermission{
				1: {
					"managed:users:1:permissions": {{Action: "teams:read", Scope: team1Scope}},
				},
			},
		},
		{
			desc: "inherit permissions from managed role",
			putRolePerms: map[int64]map[string][]rawPermission{
				1: {
					"managed:builtins:viewer:permissions": {
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
					},
					"managed:builtins:editor:permissions": {
						{Action: "teams:delete", Scope: team1Scope},
					},
				},
				2: {
					"managed:users:1:permissions": {{Action: "teams:read", Scope: team1Scope}},
					"managed:builtins:viewer:permissions": {
						{Action: "teams:delete", Scope: team1Scope},
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
					},
					"managed:builtins:editor:permissions": {
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
					},
					"managed:builtins:admin:permissions": {
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
						{Action: "teams:write", Scope: team1Scope},
					},
				},
			},
			wantRolePerms: map[int64]map[string][]rawPermission{
				1: {
					"managed:builtins:viewer:permissions": {
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
					},
					"managed:builtins:editor:permissions": {
						{Action: "teams:delete", Scope: team1Scope},
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
					},
					"managed:builtins:admin:permissions": {
						{Action: "teams:delete", Scope: team1Scope},
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
					},
				},
				2: {
					"managed:users:1:permissions": {{Action: "teams:read", Scope: team1Scope}},
					"managed:builtins:viewer:permissions": {
						{Action: "teams:delete", Scope: team1Scope},
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
					},
					"managed:builtins:editor:permissions": {
						{Action: "teams:delete", Scope: team1Scope},
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
					},
					"managed:builtins:admin:permissions": {
						{Action: "teams:delete", Scope: team1Scope},
						{Action: "teams.permissions:read", Scope: team1Scope},
						{Action: "teams.permissions:write", Scope: team2Scope},
						{Action: "teams:write", Scope: team1Scope},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Remove migration
			_, errDeleteMig := x.Exec(`DELETE FROM migration_log WHERE migration_id = ?;
DELETE FROM permission; DELETE FROM role`, acmig.ManagedPermissionsMigrationID)
			require.NoError(t, errDeleteMig)

			// put permissions
			putTestPermissions(t, x, tc.putRolePerms)

			// Run accesscontrol migration (permissions insertion should not have conflicted)
			acmigrator := migrator.NewMigrator(x, &setting.Cfg{Logger: log.New("acmigration.test")})
			acmig.AddManagedPermissionsMigration(acmigrator, acmig.ManagedPermissionsMigrationID)

			errRunningMig := acmigrator.Start(false, 0)
			require.NoError(t, errRunningMig)

			// verify got == want
			for orgID, roles := range tc.wantRolePerms {
				for roleName := range roles {
					// Check managed roles exist
					role := accesscontrol.Role{}
					hasRole, errManagedRoleSearch := x.Table("role").Where("org_id = ? AND name = ?", orgID, roleName).Get(&role)

					require.NoError(t, errManagedRoleSearch)
					require.True(t, hasRole, "expected role to exist", "orgID", orgID, "role", roleName)

					// Check permissions associated with each role
					perms := []accesscontrol.Permission{}
					count, errManagedPermsSearch := x.Table("permission").Where("role_id = ?", role.ID).FindAndCount(&perms)

					require.NoError(t, errManagedPermsSearch)
					require.Equal(t, int64(len(tc.wantRolePerms[orgID][roleName])), count, "expected role to be tied to permissions", "orgID", orgID, "role", roleName)

					gotRawPerms := convertToRawPermissions(perms)
					require.ElementsMatch(t, gotRawPerms, tc.wantRolePerms[orgID][roleName], "expected role to have permissions", "orgID", orgID, "role", roleName)

					// Check assignment of the roles
					br := accesscontrol.BuiltinRole{}
					has, errAssignmentSearch := x.Table("builtin_role").Where("role_id = ? AND role = ? AND org_id = ?", role.ID, acmig.ParseRoleFromName(roleName), orgID).Get(&br)
					require.NoError(t, errAssignmentSearch)
					require.True(t, has, "expected assignment of role to builtin role", "orgID", orgID, "role", roleName)
				}
			}
		})
	}
}

func putTestPermissions(t *testing.T, x *xorm.Engine, rolePerms map[int64]map[string][]rawPermission) {
	for orgID, roles := range rolePerms {
		for roleName, perms := range roles {
			uid := strconv.FormatInt(orgID, 10) + strings.ReplaceAll(roleName, ":", "_")
			role := accesscontrol.Role{
				OrgID:   orgID,
				Version: 1,
				UID:     uid,
				Name:    roleName,
				Updated: now,
				Created: now,
			}
			roleCount, errInsertRole := x.Insert(&role)
			require.NoError(t, errInsertRole)
			require.Equal(t, int64(1), roleCount)

			br := accesscontrol.BuiltinRole{
				RoleID:  role.ID,
				OrgID:   role.OrgID,
				Role:    acmig.ParseRoleFromName(roleName),
				Updated: now,
				Created: now,
			}
			brCount, err := x.Insert(br)
			require.NoError(t, err)
			require.Equal(t, int64(1), brCount)

			permissions := []accesscontrol.Permission{}
			for _, p := range perms {
				permissions = append(permissions, p.toPermission(role.ID, now))
			}
			permissionsCount, err := x.Insert(permissions)
			require.NoError(t, err)
			require.Equal(t, int64(len(perms)), permissionsCount)
		}
	}
}
