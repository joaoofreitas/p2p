package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// Start periodic discovery and connection maintenance
func (node *P2PNode) StartDiscoveryAndMaintenance() {
	// Start periodic discovery
	go node.periodicDiscovery()
	
	// Start connection maintenance
	go node.connectionMaintenance()
}

// Periodic discovery and peer list requests
func (node *P2PNode) periodicDiscovery() {
	ticker := time.NewTicker(node.DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !node.IsRunning {
				return
			}
			node.requestPeerLists()
			node.tryConnectToKnownPeers()
		}
	}
}

// Request peer lists from connected peers
func (node *P2PNode) requestPeerLists() {
	node.mu.RLock()
	defer node.mu.RUnlock()

	for _, peer := range node.Peers {
		if peer.Conn != nil {
			fmt.Fprintf(peer.Conn, "REQUEST_PEERS\n")
		}
	}
}

// Try to connect to known but unconnected peers
func (node *P2PNode) tryConnectToKnownPeers() {
	node.mu.RLock()
	node.mu.RLock()

	// Find unconnected known peers
	var candidates []string
	for address := range node.KnownPeers {
		if node.isMyAddress(address) {
			continue
		}

		// Check if already connected
		connected := false
		for _, peer := range node.Peers {
			if peer.Address == address {
				connected = true
				break
			}
		}

		if !connected && len(node.Peers) < node.MaxPeers {
			candidates = append(candidates, address)
		}
	}

	node.mu.RUnlock()
	node.mu.RUnlock()

	// Try to connect to a few random candidates
	if len(candidates) > 0 {
		// Shuffle and pick up to 3 candidates
		rand.Shuffle(len(candidates), func(i, j int) {
			candidates[i], candidates[j] = candidates[j], candidates[i]
		})

		limit := len(candidates)
		if limit > 3 {
			limit = 3
		}

		for i := 0; i < limit; i++ {
			address := candidates[i]
			go func(addr string) {
				// Random delay to avoid connection storms
				time.Sleep(time.Duration(rand.Intn(5000)) * time.Millisecond)
				node.ConnectToPeer(addr)
			}(address)
		}
	}
}

// Connection maintenance - remove dead connections
func (node *P2PNode) connectionMaintenance() {
	ticker := time.NewTicker(node.MaintenanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !node.IsRunning {
				return
			}
			node.pingPeers()
			node.cleanupDeadConnections()
		}
	}
}

// Ping all connected peers
func (node *P2PNode) pingPeers() {
	node.mu.RLock()
	defer node.mu.RUnlock()

	for _, peer := range node.Peers {
		if peer.Conn != nil {
			node.sendDebugMessage(MsgDebugPing, map[string]interface{}{
				"to": peer.Name,
			})
			fmt.Fprintf(peer.Conn, "PING\n")
		}
	}
}

// Clean up connections that haven't been seen recently
func (node *P2PNode) cleanupDeadConnections() {
	node.mu.Lock()
	defer node.mu.Unlock()

	now := time.Now()
	for id, peer := range node.Peers {
		if now.Sub(peer.LastSeen) > node.PeerTimeout {
			node.sendMessage(MsgMaintenanceCleanup, map[string]interface{}{
				"peerID":   id,
				"peerName": peer.Name,
				"reason":   "timeout",
			})
			peer.Conn.Close()
			delete(node.Peers, id)
		}
	}
}

// Bootstrap by connecting to a known peer
func (node *P2PNode) Bootstrap() {
	if node.BootstrapIP != "" && node.BootstrapPort > 0 {
		address := fmt.Sprintf("%s:%d", node.BootstrapIP, node.BootstrapPort)
		// Bootstrap attempt is silent in library mode

		// Retry bootstrap with exponential backoff
		for i := 0; i < 5; i++ {
			time.Sleep(time.Duration(1<<i) * time.Second) // 1, 2, 4, 8, 16 seconds

			err := node.ConnectToPeer(address)
			if err == nil {
				node.sendMessage(MsgBootstrapSuccess, map[string]interface{}{
					"address": address,
				})
				return
			}

			// Bootstrap failures are handled silently
		}

		node.sendMessage(MsgBootstrapFailed, map[string]interface{}{
			"address": address,
			"attempts": 5,
		})
	}
}

// Handle received peer list with better deduplication
func (node *P2PNode) handlePeerList(peerListStr string, fromPeerID string) {
	if peerListStr == "" {
		return
	}

	addresses := strings.Split(peerListStr, ",")
	newPeers := 0

	node.mu.Lock()
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address == "" || node.isMyAddress(address) {
			continue
		}

		// Add to known peers if new
		if _, exists := node.KnownPeers[address]; !exists {
			node.KnownPeers[address] = time.Now()
			newPeers++
		}
	}
	node.mu.Unlock()

	if newPeers > 0 {
		node.sendMessage(MsgPeersDiscovered, map[string]interface{}{
			"newCount":   newPeers,
			"fromPeer":   fromPeerID,
			"totalKnown": len(node.KnownPeers),
		})
		
		// Immediately try to connect to newly discovered peers
		go node.tryConnectToKnownPeers()
	}
}

// Share our peer list and known peers
func (node *P2PNode) sharePeerList(targetPeer *Peer) {
	node.mu.RLock()
	node.mu.RLock()

	var addresses []string

	// Add connected peers
	for _, peer := range node.Peers {
		if peer.ID != targetPeer.ID && peer.Conn != nil {
			addresses = append(addresses, peer.Address)
		}
	}

	// Add some known peers (not necessarily connected)
	count := 0
	for address := range node.KnownPeers {
		if count >= 5 { // Limit to avoid huge messages
			break
		}
		if !node.isMyAddress(address) {
			// Check if already in the list
			found := false
			for _, addr := range addresses {
				if addr == address {
					found = true
					break
				}
			}
			if !found {
				addresses = append(addresses, address)
				count++
			}
		}
	}

	node.mu.RUnlock()
	node.mu.RUnlock()

	if len(addresses) > 0 {
		peerList := strings.Join(addresses, ",")
		fmt.Fprintf(targetPeer.Conn, "PEERS:%s\n", peerList)
	}
}