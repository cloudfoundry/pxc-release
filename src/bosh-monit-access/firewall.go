//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

const (
	TableName          = "bosh_agent"
	MonitJobsChainName = "monit_access_jobs"
	MonitPort          = 2822
	LogPrefix          = "bosh-monit-access: "
)

// jobsChainExists checks if the new nftables firewall with monit_access_jobs chain exists.
// Returns true if both the table "inet bosh_agent" and chain "monit_access_jobs" exist.
func jobsChainExists() bool {
	conn, err := nftables.New()
	if err != nil {
		return false
	}
	defer conn.CloseLasting()

	// List all tables to find bosh_agent
	tables, err := conn.ListTables()
	if err != nil {
		return false
	}

	var boshTable *nftables.Table
	for _, t := range tables {
		if t.Name == TableName && t.Family == nftables.TableFamilyINet {
			boshTable = t
			break
		}
	}
	if boshTable == nil {
		return false
	}

	// List all chains to find monit_access_jobs
	chains, err := conn.ListChains()
	if err != nil {
		return false
	}

	for _, c := range chains {
		if c.Table.Name == TableName && c.Name == MonitJobsChainName {
			return true
		}
	}

	return false
}

// addCgroupRule adds a cgroup-based firewall rule to the monit_access_jobs chain.
// Uses the cgroup inode ID for matching (required by nftables kernel).
func addCgroupRule(inodeID uint64) error {
	conn, err := nftables.New()
	if err != nil {
		return fmt.Errorf("creating nftables connection: %w", err)
	}
	defer conn.CloseLasting()

	table := &nftables.Table{
		Family: nftables.TableFamilyINet,
		Name:   TableName,
	}

	chain := &nftables.Chain{
		Name:  MonitJobsChainName,
		Table: table,
	}

	// Check if rule already exists (idempotency)
	rules, err := conn.GetRules(table, chain)
	if err == nil {
		for _, rule := range rules {
			if ruleMatchesCgroup(rule, inodeID) {
				fmt.Println("bosh-monit-access: Cgroup rule already exists, skipping")
				return nil
			}
		}
	}

	// Build rule expressions:
	// socket cgroupv2 level 2 <inode-id> ip daddr 127.0.0.1 tcp dport 2822 log prefix "..." accept
	exprs := buildCgroupMatchExprs(inodeID)
	exprs = append(exprs, buildLoopbackDestExprs()...)
	exprs = append(exprs, buildTCPDestPortExprs(MonitPort)...)
	exprs = append(exprs, buildLogExpr(LogPrefix+"cgroup match: ")...)
	exprs = append(exprs, &expr.Verdict{Kind: expr.VerdictAccept})

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: exprs,
	})

	if err := conn.Flush(); err != nil {
		return fmt.Errorf("flushing nftables rules: %w", err)
	}

	return nil
}

// addUIDRule adds a UID-based firewall rule to the monit_access_jobs chain.
// This is the fallback when cgroup matching is not available.
func addUIDRule(uid uint32) error {
	conn, err := nftables.New()
	if err != nil {
		return fmt.Errorf("creating nftables connection: %w", err)
	}
	defer conn.CloseLasting()

	table := &nftables.Table{
		Family: nftables.TableFamilyINet,
		Name:   TableName,
	}

	chain := &nftables.Chain{
		Name:  MonitJobsChainName,
		Table: table,
	}

	// Check if rule already exists (idempotency)
	rules, err := conn.GetRules(table, chain)
	if err == nil {
		for _, rule := range rules {
			if ruleMatchesUID(rule, uid) {
				fmt.Println("bosh-monit-access: UID rule already exists, skipping")
				return nil
			}
		}
	}

	// Build rule expressions:
	// meta skuid <uid> ip daddr 127.0.0.1 tcp dport 2822 log prefix "..." accept
	exprs := buildUIDMatchExprs(uid)
	exprs = append(exprs, buildLoopbackDestExprs()...)
	exprs = append(exprs, buildTCPDestPortExprs(MonitPort)...)
	exprs = append(exprs, buildLogExpr(fmt.Sprintf(LogPrefix+"UID %d match: ", uid))...)
	exprs = append(exprs, &expr.Verdict{Kind: expr.VerdictAccept})

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: exprs,
	})

	if err := conn.Flush(); err != nil {
		return fmt.Errorf("flushing nftables rules: %w", err)
	}

	return nil
}

// buildCgroupMatchExprs creates nftables expressions for cgroupv2 socket matching.
// Uses 8-byte inode ID (not path string) as required by kernel.
func buildCgroupMatchExprs(inodeID uint64) []expr.Any {
	// Convert inode ID to 8-byte array in native byte order
	inodeIDBytes := make([]byte, 8)
	binary.NativeEndian.PutUint64(inodeIDBytes, inodeID)

	return []expr.Any{
		&expr.Socket{
			Key:      expr.SocketKeyCgroupv2,
			Level:    2, // Hardcoded level 2 for systemd scope nesting
			Register: 1,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     inodeIDBytes,
		},
	}
}

// buildUIDMatchExprs creates expressions for matching socket UID.
func buildUIDMatchExprs(uid uint32) []expr.Any {
	uidBytes := make([]byte, 4)
	binary.NativeEndian.PutUint32(uidBytes, uid)

	return []expr.Any{
		&expr.Meta{
			Key:      expr.MetaKeySKUID,
			Register: 1,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     uidBytes,
		},
	}
}

// buildLoopbackDestExprs creates expressions for matching IPv4 loopback destination (127.0.0.1).
func buildLoopbackDestExprs() []expr.Any {
	return []expr.Any{
		// Check this is IPv4
		&expr.Meta{
			Key:      expr.MetaKeyNFPROTO,
			Register: 1,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     []byte{unix.NFPROTO_IPV4},
		},
		// Load destination IP
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       16, // Destination IP offset in IPv4 header
			Len:          4,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     net.ParseIP("127.0.0.1").To4(),
		},
	}
}

// buildTCPDestPortExprs creates expressions for matching TCP destination port.
func buildTCPDestPortExprs(port int) []expr.Any {
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))

	return []expr.Any{
		&expr.Meta{
			Key:      expr.MetaKeyL4PROTO,
			Register: 1,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     []byte{unix.IPPROTO_TCP},
		},
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseTransportHeader,
			Offset:       2, // Destination port offset in TCP header
			Len:          2,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     portBytes,
		},
	}
}

// buildLogExpr creates a log expression with the given prefix.
func buildLogExpr(prefix string) []expr.Any {
	return []expr.Any{
		&expr.Log{
			Key:  1 << unix.NFTA_LOG_PREFIX,
			Data: []byte(prefix),
		},
	}
}

// ruleMatchesCgroup checks if an existing rule matches the given cgroup inode ID.
func ruleMatchesCgroup(rule *nftables.Rule, inodeID uint64) bool {
	hasSocketExpr := false
	hasCgroupMatch := false

	for _, e := range rule.Exprs {
		if socketExpr, ok := e.(*expr.Socket); ok {
			if socketExpr.Key == expr.SocketKeyCgroupv2 {
				hasSocketExpr = true
			}
		}
		if cmpExpr, ok := e.(*expr.Cmp); ok {
			if len(cmpExpr.Data) == 8 {
				existingID := binary.NativeEndian.Uint64(cmpExpr.Data)
				if existingID == inodeID {
					hasCgroupMatch = true
				}
			}
		}
	}

	return hasSocketExpr && hasCgroupMatch
}

// ruleMatchesUID checks if an existing rule matches the given UID.
func ruleMatchesUID(rule *nftables.Rule, uid uint32) bool {
	hasMetaSKUID := false
	hasUIDMatch := false

	for _, e := range rule.Exprs {
		if metaExpr, ok := e.(*expr.Meta); ok {
			if metaExpr.Key == expr.MetaKeySKUID {
				hasMetaSKUID = true
			}
		}
		if cmpExpr, ok := e.(*expr.Cmp); ok {
			if len(cmpExpr.Data) == 4 {
				existingUID := binary.NativeEndian.Uint32(cmpExpr.Data)
				if existingUID == uid {
					hasUIDMatch = true
				}
			}
		}
	}

	return hasMetaSKUID && hasUIDMatch
}
