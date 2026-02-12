//go:build linux

// bosh-monit-access is a helper binary for BOSH jobs to add monit firewall rules
// to the new nftables-based firewall implemented in the bosh-agent.
//
// Usage:
//
//	bosh-monit-access --check    # Check if new firewall is available (exit 0 = yes)
//	bosh-monit-access            # Add firewall rule (cgroup preferred, UID fallback)
//
// This binary serves as a replacement for the complex bash firewall setup logic
// that was previously in job service scripts.
package main

import (
	"fmt"
	"os"
)

func main() {
	// Check mode: verify if new firewall with jobs chain exists
	if len(os.Args) > 1 && os.Args[1] == "--check" {
		if jobsChainExists() {
			os.Exit(0)
		}
		os.Exit(1)
	}

	// Setup mode: add firewall rule
	fmt.Println("bosh-monit-access: Setting up monit firewall rule")

	// 1. Check if jobs chain exists
	if !jobsChainExists() {
		fmt.Println("bosh-monit-access: monit_access_jobs chain not found (old stemcell), skipping")
		os.Exit(0)
	}

	// 2. Try cgroup-based rule first (better isolation)
	cgroupPath, err := getCurrentCgroupPath()
	if err == nil && isCgroupAccessible(cgroupPath) {
		inodeID, err := getCgroupInodeID(cgroupPath)
		if err == nil {
			fmt.Printf("bosh-monit-access: Using cgroup rule for: %s (inode: %d)\n", cgroupPath, inodeID)

			if err := addCgroupRule(inodeID, cgroupPath); err == nil {
				fmt.Println("bosh-monit-access: Successfully added cgroup-based rule")
				os.Exit(0)
			} else {
				fmt.Printf("bosh-monit-access: Failed to add cgroup rule: %v\n", err)
			}
		} else {
			fmt.Printf("bosh-monit-access: Failed to get cgroup inode ID: %v\n", err)
		}
	} else if err != nil {
		fmt.Printf("bosh-monit-access: Could not detect cgroup: %v\n", err)
	}

	// 3. Fallback to UID-based rule
	uid := uint32(os.Getuid())
	fmt.Printf("bosh-monit-access: Falling back to UID rule for UID: %d\n", uid)

	if err := addUIDRule(uid); err != nil {
		fmt.Fprintf(os.Stderr, "bosh-monit-access: FAILED to add firewall rule: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("bosh-monit-access: Successfully added UID-based rule")
}
