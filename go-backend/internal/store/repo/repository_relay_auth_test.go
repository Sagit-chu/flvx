package repo

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"

	"go-backend/internal/relaysecurity"
	"go-backend/internal/store/model"
)

func TestRelayEndpointCredentialsBindsLegacyChainTypeAsText(t *testing.T) {
	for _, role := range []int{2, 3} {
		t.Run(strconv.Itoa(role), func(t *testing.T) {
			r, err := Open(filepath.Join(t.TempDir(), "relay.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = r.Close() })

			// SQLite coerces integer parameters for this legacy text column.
			// Check the actual repository query with pgx's text encoder as well,
			// so the PostgreSQL regression is caught without a running server.
			checked := 0
			if err := r.DB().Callback().Query().After("gorm:query").Register("test:relay_chain_text_parameter", func(db *gorm.DB) {
				if db.Statement.Table != "chain_tunnel" {
					return
				}
				checked++
				if len(db.Statement.Vars) != 3 {
					db.AddError(fmt.Errorf("expected three chain lookup arguments, got %d", len(db.Statement.Vars)))
					return
				}
				encoded, err := pgtype.NewMap().Encode(pgtype.TextOID, pgtype.TextFormatCode, db.Statement.Vars[2], nil)
				if err != nil {
					db.AddError(fmt.Errorf("encode chain_type for PostgreSQL: %w", err))
				} else if string(encoded) != strconv.Itoa(role) {
					db.AddError(fmt.Errorf("unexpected chain_type argument: %q", encoded))
				}
			}); err != nil {
				t.Fatal(err)
			}

			tunnel := model.Tunnel{Name: "relay", Type: 2, Flow: 1, RelayAuthMode: relaysecurity.Required}
			if err := r.DB().Create(&tunnel).Error; err != nil {
				t.Fatal(err)
			}
			const nodeID = int64(7)
			if err := r.DB().Transaction(func(tx *gorm.DB) error {
				created, err := r.RelayEndpointCredentials(tx, tunnel.ID, nodeID, role, true)
				if err != nil {
					return err
				}
				for _, create := range []bool{false, true} {
					loaded, err := r.RelayEndpointCredentials(tx, tunnel.ID, nodeID, role, create)
					if err != nil {
						return err
					}
					if *created != *loaded {
						return fmt.Errorf("reloading endpoint changed credentials")
					}
				}
				return nil
			}); err != nil {
				t.Fatalf("create and reload endpoint: %v", err)
			}
			if checked != 1 {
				t.Fatalf("expected one chain lookup for a new endpoint, got %d", checked)
			}

			chain := model.ChainTunnel{TunnelID: tunnel.ID, NodeID: nodeID, ChainType: strconv.Itoa(role)}
			if err := r.DB().Create(&chain).Error; err != nil {
				t.Fatal(err)
			}
			if err := r.DB().Where("tunnel_id = ?", tunnel.ID).Delete(&model.RelayEndpointAuth{}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := r.RelayEndpointCredentials(nil, tunnel.ID, nodeID, role, true); err == nil || !strings.Contains(err.Error(), "refusing regeneration") {
				t.Fatalf("missing protected credentials must fail closed, got %v", err)
			}
			if checked != 2 {
				t.Fatalf("expected chain lookup before refusing regeneration, got %d", checked)
			}
			var count int64
			if err := r.DB().Model(&model.RelayEndpointAuth{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("regenerated credentials for an existing protected endpoint")
			}
		})
	}
}
