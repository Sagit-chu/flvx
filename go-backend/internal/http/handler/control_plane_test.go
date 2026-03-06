package handler

import (
	"errors"
	"reflect"
	"testing"

	"go-backend/internal/store/repo"
)

func TestBuildForwardControlServiceNamesPauseResume(t *testing.T) {
	base := "12_34_56"
	want := []string{base + "_tcp", base + "_udp"}

	for _, command := range []string{"PauseService", "ResumeService"} {
		got := buildForwardControlServiceNames(base, command)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("command %s expected %v, got %v", command, want, got)
		}
	}
}

func TestBuildForwardControlServiceNamesDelete(t *testing.T) {
	base := "12_34_56"
	want := []string{base, base + "_tcp", base + "_udp"}
	got := buildForwardControlServiceNames(base, " DeleteService ")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildForwardServiceBaseCandidates(t *testing.T) {
	got := buildForwardServiceBaseCandidates(12, 34, 56, []int64{56, 78, 90})
	want := []string{"12_34_56", "12_34_78", "12_34_90", "12_34_0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildForwardServiceBaseCandidatesWithZeroPreferred(t *testing.T) {
	got := buildForwardServiceBaseCandidates(12, 34, 0, []int64{78, 0, 90})
	want := []string{"12_34_0", "12_34_78", "12_34_90"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBuildForwardServiceBaseWithResolvedUserTunnel(t *testing.T) {
	got := buildForwardServiceBaseWithResolvedUserTunnel(12, 34, 56)
	if got != "12_34_56" {
		t.Fatalf("expected 12_34_56, got %s", got)
	}
}

func TestBuildForwardServiceBaseWithResolvedUserTunnelFallbackToZero(t *testing.T) {
	got := buildForwardServiceBaseWithResolvedUserTunnel(12, 34, 0)
	if got != "12_34_0" {
		t.Fatalf("expected 12_34_0, got %s", got)
	}
}

func TestShouldTryLegacySingleService(t *testing.T) {
	if !shouldTryLegacySingleService("PauseService") {
		t.Fatalf("PauseService should require legacy fallback")
	}
	if !shouldTryLegacySingleService("resumeService") {
		t.Fatalf("ResumeService should require legacy fallback")
	}
	if shouldTryLegacySingleService("DeleteService") {
		t.Fatalf("DeleteService should not require legacy fallback")
	}
}

func TestShouldSelfHealForwardServiceControl(t *testing.T) {
	if !shouldSelfHealForwardServiceControl("PauseService") {
		t.Fatalf("PauseService should trigger self-heal")
	}
	if !shouldSelfHealForwardServiceControl(" resumeService ") {
		t.Fatalf("ResumeService should trigger self-heal")
	}
	if shouldSelfHealForwardServiceControl("DeleteService") {
		t.Fatalf("DeleteService should not trigger self-heal")
	}
}

func TestControlForwardServiceCommandHandledOnKnownVariant(t *testing.T) {
	bases := []string{"12_34_56"}
	called := make([]string, 0)
	handled, lastNotFoundErr, err := controlForwardServiceCommand(bases, "PauseService", func(name string) error {
		called = append(called, name)
		if name == "12_34_56_udp" {
			return nil
		}
		return errors.New("service " + name + " not found")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if lastNotFoundErr != nil {
		t.Fatalf("expected lastNotFoundErr=nil when handled")
	}
	wantCalls := []string{"12_34_56_tcp", "12_34_56_udp", "12_34_56"}
	if !reflect.DeepEqual(called, wantCalls) {
		t.Fatalf("expected calls %v, got %v", wantCalls, called)
	}
}

func TestControlForwardServiceCommandReturnsLastNotFoundWhenAllMissing(t *testing.T) {
	bases := []string{"12_34_56"}
	handled, lastNotFoundErr, err := controlForwardServiceCommand(bases, "PauseService", func(name string) error {
		return errors.New("service " + name + " not found")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatalf("expected handled=false")
	}
	if lastNotFoundErr == nil {
		t.Fatalf("expected lastNotFoundErr when all variants are missing")
	}
}

func TestDeleteForwardServiceCandidatesSkipsNotFoundUntilLegacyMatch(t *testing.T) {
	bases := []string{"12_34_56", "12_34_0"}
	called := make([]string, 0)
	err := deleteForwardServiceCandidates(bases, func(name string) error {
		called = append(called, name)
		if name == "12_34_0" {
			return nil
		}
		return errors.New("service " + name + " not found")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantCalls := []string{"12_34_56_tcp", "12_34_56_udp", "12_34_56", "12_34_0_tcp", "12_34_0_udp", "12_34_0"}
	if !reflect.DeepEqual(called, wantCalls) {
		t.Fatalf("expected calls %v, got %v", wantCalls, called)
	}
}

func TestDeleteForwardServiceCandidatesTreatsAllMissingAsSuccess(t *testing.T) {
	bases := []string{"12_34_56", "12_34_0"}
	err := deleteForwardServiceCandidates(bases, func(name string) error {
		return errors.New("service " + name + " not found")
	})
	if err != nil {
		t.Fatalf("all-missing delete should be tolerated, got %v", err)
	}
}

func TestForwardServiceBaseCandidatesIncludesResolvedAndLegacyZero(t *testing.T) {
	bases := buildForwardServiceBaseCandidates(46, 9, 123, []int64{123, 77, 0})
	want := []string{"46_9_123", "46_9_77", "46_9_0"}
	if !reflect.DeepEqual(bases, want) {
		t.Fatalf("expected %v, got %v", want, bases)
	}
}

func TestDeleteForwardServiceBasesOnNodeRetriesLegacyZeroResidue(t *testing.T) {
	bases := []string{"46_9_123", "46_9_0"}
	called := make([]string, 0)
	err := deleteForwardServiceCandidates(bases, func(name string) error {
		called = append(called, name)
		if name == "46_9_0_tcp" || name == "46_9_0_udp" {
			return nil
		}
		return errors.New("service " + name + " not found")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"46_9_123_tcp", "46_9_123_udp", "46_9_123", "46_9_0_tcp", "46_9_0_udp", "46_9_0"}
	if !reflect.DeepEqual(called, want) {
		t.Fatalf("expected calls %v, got %v", want, called)
	}
}

func TestValidateForwardPortAvailabilityRejectsOtherForwardOccupancy(t *testing.T) {
	h := &Handler{repo: nil}
	node := &nodeRecord{ID: 9, Name: "test-node"}
	_ = h
	_ = node

	rawRepo, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	h = &Handler{repo: rawRepo}
	if err := rawRepo.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(1, 9, 2000)`).Error; err != nil {
		t.Fatalf("insert forward port: %v", err)
	}

	err = h.validateForwardPortAvailability(&nodeRecord{ID: 9, Name: "test-node"}, 2000, 2)
	if err == nil {
		t.Fatalf("expected occupancy error")
	}
	if err.Error() != "节点 test-node 端口 2000 已被其他转发占用" {
		t.Fatalf("unexpected error: %v", err)
	}

	err = h.validateForwardPortAvailability(&nodeRecord{ID: 9, Name: "test-node"}, 2000, 1)
	if err != nil {
		t.Fatalf("same forward should be allowed, got %v", err)
	}
}

func TestControlForwardServiceCommandReturnsHardError(t *testing.T) {
	bases := []string{"12_34_56"}
	handled, lastNotFoundErr, err := controlForwardServiceCommand(bases, "PauseService", func(name string) error {
		if name == "12_34_56_tcp" {
			return errors.New("network timeout")
		}
		return nil
	})
	if err == nil {
		t.Fatalf("expected hard error")
	}
	if handled {
		t.Fatalf("expected handled=false on hard error")
	}
	if lastNotFoundErr != nil {
		t.Fatalf("did not expect not-found error alongside hard error")
	}
}

func TestIsAlreadyExistsMessage(t *testing.T) {
	if !isAlreadyExistsMessage("service demo already exists") {
		t.Fatalf("expected already exists message to be tolerated")
	}
	if !isAlreadyExistsMessage("服务已存在") {
		t.Fatalf("expected Chinese already exists message to be tolerated")
	}
	if isAlreadyExistsMessage("listen tcp [::]:10001: bind: address already in use") {
		t.Fatalf("address already in use must not be treated as already exists")
	}
}

func TestIsBindAddressInUseError(t *testing.T) {
	if !isBindAddressInUseError(errors.New("listen tcp [::]:10001: bind: address already in use")) {
		t.Fatalf("address already in use should be detected")
	}
	if !isBindAddressInUseError(errors.New("listen tcp4 13.228.170.187:16765: bind: cannot assign requested address")) {
		t.Fatalf("cannot assign requested address should be detected")
	}
	if isBindAddressInUseError(errors.New("service demo already exists")) {
		t.Fatalf("already exists should not be treated as bind conflict")
	}
	if isBindAddressInUseError(nil) {
		t.Fatalf("nil error should not be treated as bind conflict")
	}
}

func TestIsAddressAlreadyInUseError(t *testing.T) {
	if !isAddressAlreadyInUseError(errors.New("listen tcp [::]:10001: bind: address already in use")) {
		t.Fatalf("address already in use should be detected")
	}
	if isAddressAlreadyInUseError(errors.New("listen tcp4 13.228.170.187:16765: bind: cannot assign requested address")) {
		t.Fatalf("cannot assign requested address should not be treated as address-in-use")
	}
}

func TestIsCannotAssignRequestedAddressError(t *testing.T) {
	if !isCannotAssignRequestedAddressError(errors.New("listen tcp4 13.228.170.187:16765: bind: cannot assign requested address")) {
		t.Fatalf("cannot assign requested address should be detected")
	}
	if isCannotAssignRequestedAddressError(errors.New("listen tcp [::]:10001: bind: address already in use")) {
		t.Fatalf("address already in use should not be treated as cannot-assign")
	}
}

func TestBuildForwardServiceConfigs_UsesBindIPForListen(t *testing.T) {
	forward := &forwardRecord{RemoteAddr: "1.2.3.4:80", Strategy: "fifo", TunnelID: 7}
	node := &nodeRecord{TCPListenAddr: "[::]", UDPListenAddr: "[::]"}
	services := buildForwardServiceConfigs("1_2_0", forward, nil, node, 22000, "10.9.8.7", nil, false)
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
	for _, svc := range services {
		addr, _ := svc["addr"].(string)
		if addr != "10.9.8.7:22000" {
			t.Fatalf("expected bind IP address 10.9.8.7:22000, got %q", addr)
		}
	}
}

func TestBuildForwardServiceConfigs_DefaultListenAddrWhenBindIPEmpty(t *testing.T) {
	forward := &forwardRecord{RemoteAddr: "1.2.3.4:80", Strategy: "fifo", TunnelID: 7}
	node := &nodeRecord{TCPListenAddr: "0.0.0.0", UDPListenAddr: "[::]"}
	services := buildForwardServiceConfigs("1_2_0", forward, nil, node, 22001, "", nil, false)
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
	tcpAddr, _ := services[0]["addr"].(string)
	udpAddr, _ := services[1]["addr"].(string)
	if tcpAddr != "0.0.0.0:22001" {
		t.Fatalf("expected tcp addr 0.0.0.0:22001, got %q", tcpAddr)
	}
	if udpAddr != "[::]:22001" {
		t.Fatalf("expected udp addr [::]:22001, got %q", udpAddr)
	}
}
func TestBuildForwardServiceConfigs_BindIPAlreadyContainsPort(t *testing.T) {
	forward := &forwardRecord{RemoteAddr: "1.2.3.4:80", Strategy: "fifo", TunnelID: 7}
	node := &nodeRecord{TCPListenAddr: "[::]", UDPListenAddr: "[::]"}
	services := buildForwardServiceConfigs("1_2_0", forward, nil, node, 55555, "3.3.3.3:12345", nil, false)
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
	for _, svc := range services {
		addr, _ := svc["addr"].(string)
		if addr != "3.3.3.3:12345" {
			t.Fatalf("expected bind IP with port 3.3.3.3:12345, got %q", addr)
		}
	}
}
