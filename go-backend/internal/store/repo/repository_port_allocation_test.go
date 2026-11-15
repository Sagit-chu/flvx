package repo

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go-backend/internal/store/model"
	"gorm.io/gorm"
)

func portTestCreate(r *Repository, port int, nodes ...int64) (int64, error) {
	return r.CreateForwardTx(1, "admin", "concurrent", 1, "127.0.0.1:80", "fifo", time.Now().UnixMilli(), 0, nodes, port, "", nil, 0, 0, nil, 0, 0, 0, true)
}
func TestAtomicPortAllocationAcrossRepositories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	runConcurrentPortAllocation(t, a, b)
}

func runConcurrentPortAllocation(t *testing.T, a, b *Repository) {
	for _, id := range []int64{1, 2} {
		if err := a.db.Create(&model.Node{ID: id, Name: fmt.Sprint(id), Port: "20000-20031"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	const count = 16
	start := make(chan struct{})
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r := a
			if i%2 == 1 {
				r = b
			}
			_, err := portTestCreate(r, 0, 1, 2)
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var rows []model.ForwardPort
	if err := a.db.Order("port").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != count*2 {
		t.Fatalf("got %d reservations", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		k := fmt.Sprintf("%d:%d", row.NodeID, row.Port)
		if seen[k] {
			t.Fatalf("duplicate %s", k)
		}
		seen[k] = true
	}
	explicitStart := make(chan struct{})
	explicitResults := make(chan error, 2)
	for _, r := range []*Repository{a, b} {
		go func(r *Repository) { <-explicitStart; _, err := portTestCreate(r, 20031, 1, 2); explicitResults <- err }(r)
	}
	close(explicitStart)
	success := 0
	for i := 0; i < 2; i++ {
		if <-explicitResults == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("explicit port race: %d successes, want exactly 1", success)
	}
	for p := 20000; p < 20000+count; p++ {
		for _, n := range []int{1, 2} {
			if !seen[fmt.Sprintf("%d:%d", n, p)] {
				t.Fatalf("gap at %d:%d", n, p)
			}
		}
	}
}
func TestAtomicPortAllocationRejectsConflictsAndExhaustion(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.db.Create(&model.Node{ID: 1, Port: "21000-21001"}).Error; err != nil {
		t.Fatal(err)
	}
	id, err := portTestCreate(r, 21000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := portTestCreate(r, 21000, 1); err == nil {
		t.Fatal("explicit collision accepted")
	}
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		return r.CreateChainTunnelTx(tx, 2, "2", 1, sql.NullInt64{Int64: 21000, Valid: true}, "", 0, "", "")
	}); err == nil {
		t.Fatal("chain collision accepted")
	}
	if _, err := portTestCreate(r, 0, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := portTestCreate(r, 0, 1); err == nil {
		t.Fatal("exhausted range accepted")
	}
	entries := []struct {
		NodeID int64
		Port   int
		InIP   string
	}{{1, 21001, ""}}
	if err := r.ReplaceForwardPorts(id, entries, true); err == nil {
		t.Fatal("replacement collision accepted")
	}
	var old model.ForwardPort
	if err := r.db.Where("forward_id = ?", id).First(&old).Error; err != nil {
		t.Fatal(err)
	}
	if old.Port != 21000 {
		t.Fatal("failed replacement lost original reservation")
	}
}

func TestAtomicPortAllocationPostgresAcrossRepositories(t *testing.T) {
	dsn := os.Getenv("FLVX_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("set FLVX_POSTGRES_TEST_DSN for isolated PostgreSQL allocation test")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("port_allocation_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	scoped := dsn + " search_path=" + schema
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		scoped = u.String()
	}
	a, err := OpenPostgres(scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenPostgres(scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	runConcurrentPortAllocation(t, a, b)
}

func TestAtomicPortAllocationRespectsRangesAndPendingCleanup(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	nodes := []model.Node{{ID: 1, Port: "23004,23000-23002", IsRemote: 1, RemoteConfig: sql.NullString{String: `{"portRangeStart":23001,"portRangeEnd":23004}`, Valid: true}}, {ID: 2, Port: "23001-23004"}}
	if err := r.db.Create(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.db.Create(&model.TunnelRuntimeCleanup{TunnelID: 2, NodeID: 2, Kind: "service", Port: 23001}).Error; err != nil {
		t.Fatal(err)
	}
	id, err := portTestCreate(r, 0, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.GetMinForwardPort(id).Int64; got != 23002 {
		t.Fatalf("got %d, want23002", got)
	}
	id2, err := portTestCreate(r, 0, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.GetMinForwardPort(id2).Int64; got != 23004 {
		t.Fatalf("got %d, want23004", got)
	}
	if err := r.DeleteForwardCascade(id); err != nil {
		t.Fatal(err)
	}
	id3, err := portTestCreate(r, 0, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.GetMinForwardPort(id3).Int64; got != 23002 {
		t.Fatalf("released gap not reused: %d", got)
	}
}

func TestAtomicPortAllocationIndependentNewEntries(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.db.Create(&[]model.Node{{ID: 1, Port: "24000-24001"}, {ID: 2, Port: "25000-25001"}}).Error; err != nil {
		t.Fatal(err)
	}
	entries := []struct {
		NodeID int64
		Port   int
		InIP   string
	}{{1, 0, ""}, {2, 0, ""}}
	if err := r.ReplaceForwardPortsIndividually(1, entries, true); err != nil {
		t.Fatal(err)
	}
	var rows []model.ForwardPort
	if err := r.db.Order("node_id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Port != 24000 || rows[1].Port != 25000 {
		t.Fatalf("unexpected independent allocation: %+v", rows)
	}
	if _, err := r.CreateFederationTunnel("conflict", 1, "tcp", 0, 1, 24000); err == nil {
		t.Fatal("federation collision accepted")
	}
}

func TestAtomicPortPeerRuntimeConflicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := a.db.Create(&model.Node{ID: 1, Port: "25000-25010"}).Error; err != nil {
		t.Fatal(err)
	}
	runtime := func(key string, port int) *model.PeerShareRuntime {
		return &model.PeerShareRuntime{ShareID: 1, NodeID: 1, Port: port, ResourceKey: key, ReservationID: key, Status: 1}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; _, err := portTestCreate(a, 25000, 1); results <- err }()
	go func() { <-start; results <- b.CreatePeerShareRuntime(runtime("peer", 25000)) }()
	close(start)
	success := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("mixed reservation race succeeded %d times", success)
	}
	first := runtime("first", 25001)
	if err := a.CreatePeerShareRuntime(first); err != nil {
		t.Fatal(err)
	}
	if err := b.CreatePeerShareRuntime(runtime("second", 25001)); err == nil {
		t.Fatal("duplicate peer port accepted")
	}
	first.Role = "middle"
	first.Applied = 1
	if err := a.UpdatePeerShareRuntime(first); err != nil {
		t.Fatalf("own metadata update: %v", err)
	}
	owned := runtime("federation-forward-1-22-25002", 25002)
	owned.Role = "forward"
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		return a.CreateChainTunnelTx(tx, 22, "1", 1, sql.NullInt64{Int64: 25002, Valid: true}, "fifo", 0, "tcp", "")
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.CreatePeerShareRuntime(runtime("federation-forward-1-22-25002", 25002)); err == nil {
		t.Fatal("generic spoof of ownership accepted")
	}
	if err := a.CreatePeerShareRuntime(owned); err != nil {
		t.Fatalf("valid attachment rejected: %v", err)
	}
	other := runtime("other-forward", 25002)
	other.Role = "forward"
	if err := b.CreatePeerShareRuntime(other); err == nil {
		t.Fatal("unowned forward attachment accepted")
	}
	pending := runtime("pending", 25003)
	pending.Status = 0
	pending.ReleasePending = 1
	if err := a.db.Create(pending).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := portTestCreate(b, 25003, 1); err == nil {
		t.Fatal("pending runtime reservation reused")
	}
}
