package repo

import (
	"path/filepath"
	"testing"
	"time"

	"go-backend/internal/store/model"
)

func TestTrafficLimitMiBSurvivesBackupRestore(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	now := time.Now().UnixMilli()
	userID, err := source.CreateUser("mib-user", "hash", 1, now+86400000, 1, 1, 10, 1, 0, now, 500)
	if err != nil {
		t.Fatal(err)
	}
	tunnel := model.Tunnel{Name: "mib-tunnel", TrafficRatio: 1, Type: 1, Protocol: "tls", Flow: 1, CreatedTime: now, UpdatedTime: now, Status: 1, Inx: 1}
	if err := source.DB().Create(&tunnel).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := source.EnsureUserTunnelGrant(userID, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	grants, err := source.GetUserPackageTunnels(userID)
	if err != nil || len(grants) != 1 || grants[0].FlowMiB != 500 {
		t.Fatalf("inherited tunnel quota = %+v, err = %v", grants, err)
	}
	backup, err := source.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, user := range backup.Users {
		if user.User == "mib-user" {
			found = user.Flow == 1 && user.FlowMiB == 500
		}
	}
	if !found {
		t.Fatal("500 MiB user quota missing from backup")
	}
	if len(backup.UserTunnels) != 1 || backup.UserTunnels[0].FlowMiB != 500 {
		t.Fatalf("tunnel quota missing from backup: %+v", backup.UserTunnels)
	}

	dest, err := Open(filepath.Join(t.TempDir(), "dest.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()
	if _, err := dest.Import(backup, []string{"users", "tunnels", "userTunnels"}); err != nil {
		t.Fatal(err)
	}
	user, err := dest.GetUserByUsername("mib-user")
	if err != nil || user == nil || user.Flow != 1 || user.FlowMiB != 500 {
		t.Fatalf("restored quota = %+v, err = %v", user, err)
	}
	grants, err = dest.GetUserPackageTunnels(user.ID)
	if err != nil || len(grants) != 1 || grants[0].FlowMiB != 500 {
		t.Fatalf("restored tunnel quota = %+v, err = %v", grants, err)
	}
}
