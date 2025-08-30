package main

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Start listening for incoming connections
func (node *P2PNode) StartServer() error {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(node.Port))
	if err != nil {
		return err
	}
	node.Listener = listener
	
	node.sendMessage(MsgSystemStart, map[string]interface{}{
		"nodeID":   node.ID,
		"nodeName": node.Name,
		"port":     node.Port,
	})
	
	go func() {
		for node.IsRunning {
			conn, err := listener.Accept()
			if err != nil {
				if node.IsRunning {
					node.sendMessage(MsgError, map[string]interface{}{
					"error": err.Error(),
					"context": "accepting connection",
				})
				}
				continue
			}
			go node.handleConnection(conn)
		}
	}()
	
	return nil
}

// Handle incoming connections with better error handling
func (node *P2PNode) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set read timeout
	conn.SetReadDeadline(time.Now().Add(node.ConnectionTimeout))

	reader := bufio.NewReader(conn)

	// Read the handshake (peer ID)
	peerID, err := reader.ReadString('\n')
	if err != nil {
		node.sendMessage(MsgError, map[string]interface{}{
			"error": err.Error(),
			"context": "reading peer ID",
		})
		return
	}
	peerID = strings.TrimSpace(peerID)

	// Check if we already have this peer
	node.mu.Lock()
	if _, exists := node.Peers[peerID]; exists {
		node.mu.Unlock()
		// Silently reject duplicate connections
		fmt.Fprintf(conn, "DUPLICATE\n")
		return
	}

	// Check peer limit
	if len(node.Peers) >= node.MaxPeers {
		node.mu.Unlock()
		// Silently reject when max peers reached
		fmt.Fprintf(conn, "MAX_PEERS\n")
		return
	}
	node.mu.Unlock()

	// Send our ID:NAME back
	fmt.Fprintf(conn, "%s:%s\n", node.ID, node.Name)

	// Remove read timeout for ongoing communication
	conn.SetReadDeadline(time.Time{})

	// Parse peer ID and name (format: "ID:NAME")
	parts := strings.SplitN(peerID, ":", 2)
	actualPeerID := parts[0]
    peerName := actualPeerID // default to ID if no name
    if len(parts) == 2 && parts[1] != "" {
        peerName = parts[1]
    }


	// Add peer to our list
	peer := &Peer{
		ID:       actualPeerID,
		Name:     peerName,
		Address:  conn.RemoteAddr().String(),
		Conn:     conn,
		LastSeen: time.Now(),
	}

	node.mu.Lock()
	node.Peers[actualPeerID] = peer
	node.mu.Unlock()

	node.sendMessage(MsgPeerConnected, map[string]interface{}{
		"peerID":     actualPeerID,
		"peerName":   peerName,
		"address":    conn.RemoteAddr().String(),
		"peerCount":  len(node.Peers),
		"maxPeers":   node.MaxPeers,
		"direction":  "incoming",
	})

	// Share our peer list immediately
	node.sharePeerList(peer)

	// Listen for messages
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			node.sendMessage(MsgPeerDisconnected, map[string]interface{}{
				"peerID":   actualPeerID,
				"peerName": peerName,
			})
			break
		}
		message = strings.TrimSpace(message)

		// Update last seen time
		node.mu.Lock()
		if p, exists := node.Peers[peerID]; exists {
			p.LastSeen = time.Now()
		}
		node.mu.Unlock()

		if message == "PING" {
			node.sendDebugMessage(MsgDebugPing, map[string]interface{}{
				"from": peerName,
			})
			fmt.Fprintf(conn, "PONG\n")
			continue
		}

		if message == "REQUEST_PEERS" {
			node.sendDebugMessage(MsgDebugPeerRequest, map[string]interface{}{
				"from": peerName,
			})
			node.sharePeerList(peer)
			continue
		}

		if strings.HasPrefix(message, "PEERS:") {
			node.sendDebugMessage(MsgDebugPeerResponse, map[string]interface{}{
				"from":      peerName,
				"peerCount": len(strings.Split(message[6:], ",")),
			})
			node.handlePeerList(message[6:], actualPeerID)
			continue
		}

		node.sendMessage(MsgChatMessage, map[string]interface{}{
			"from":    peerName,
			"message": message,
		})
	}

	// Remove peer when disconnected
	node.mu.Lock()
	delete(node.Peers, actualPeerID)
	node.mu.Unlock()
}

// Connect to a peer with better error handling and deduplication
func (node *P2PNode) ConnectToPeer(address string) error {
	// Don't connect to ourselves
	if node.isMyAddress(address) {
		return fmt.Errorf("cannot connect to self")
	}

	// Check if already connected
	node.mu.RLock()
	for _, peer := range node.Peers {
		if peer.Address == address || strings.Contains(peer.Address, strings.Split(address, ":")[1]) {
			node.mu.RUnlock()
			return fmt.Errorf("already connected to %s", address)
		}
	}
	peerCount := len(node.Peers)
	node.mu.RUnlock()

	// Check peer limit
	if peerCount >= node.MaxPeers {
		return fmt.Errorf("max peers reached (%d/%d)", peerCount, node.MaxPeers)
	}

	// Add to known peers
	node.mu.Lock()
	node.KnownPeers[address] = time.Now()
	node.mu.Unlock()

	conn, err := net.DialTimeout("tcp", address, node.ConnectionTimeout)
	if err != nil {
		return err
	}

	// Send handshake (our ID:NAME)
	fmt.Fprintf(conn, "%s:%s\n", node.ID, node.Name)

	// Read peer's response
	conn.SetReadDeadline(time.Now().Add(node.ConnectionTimeout))
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return err
	}
	response = strings.TrimSpace(response)

	// Handle rejection responses
	if response == "DUPLICATE" {
		conn.Close()
		return fmt.Errorf("peer rejected: duplicate connection")
	}
	if response == "MAX_PEERS" {
		conn.Close()
		return fmt.Errorf("peer rejected: max peers reached")
	}

	// Parse peer ID and name from response (format: "ID:NAME")
	parts := strings.SplitN(response, ":", 2)
	actualPeerID := parts[0]
	peerName := actualPeerID // default to ID if no name
	if len(parts) == 2 && parts[1] != "" {
		peerName = parts[1]
	}
	
	conn.SetReadDeadline(time.Time{})

	// Add peer to our list
	peer := &Peer{
		ID:       actualPeerID,
		Name:     peerName,
		Address:  address,
		Conn:     conn,
		LastSeen: time.Now(),
	}

	node.mu.Lock()
	node.Peers[actualPeerID] = peer
	peerCount = len(node.Peers)
	node.mu.Unlock()

	node.sendMessage(MsgPeerConnected, map[string]interface{}{
		"peerID":     actualPeerID,
		"peerName":   peerName,
		"address":    address,
		"peerCount":  peerCount,
		"maxPeers":   node.MaxPeers,
		"direction":  "outgoing",
	})

	// Share our peer list immediately and request theirs
	node.sharePeerList(peer)
	fmt.Fprintf(conn, "REQUEST_PEERS\n")

	// Start listening for messages from this peer
	go func() {
		defer conn.Close()
		for {
			message, err := reader.ReadString('\n')
			if err != nil {
				node.sendMessage(MsgPeerDisconnected, map[string]interface{}{
					"peerID":   actualPeerID,
					"peerName": peerName,
				})
				break
			}
			message = strings.TrimSpace(message)

			// Update last seen time
			node.mu.Lock()
			if p, exists := node.Peers[actualPeerID]; exists {
				p.LastSeen = time.Now()
			}
			node.mu.Unlock()

			if message == "PONG" {
				node.sendDebugMessage(MsgDebugPong, map[string]interface{}{
					"from": peerName,
				})
				continue
			}

			if message == "REQUEST_PEERS" {
				node.sendDebugMessage(MsgDebugPeerRequest, map[string]interface{}{
					"from": peerName,
				})
				node.sharePeerList(peer)
				continue
			}

			if strings.HasPrefix(message, "PEERS:") {
				node.sendDebugMessage(MsgDebugPeerResponse, map[string]interface{}{
					"from":      peerName,
					"peerCount": len(strings.Split(message[6:], ",")),
				})
				node.handlePeerList(message[6:], actualPeerID)
				continue
			}

			node.sendMessage(MsgChatMessage, map[string]interface{}{
				"from":    peerName,
				"message": message,
			})
		}

		// Remove peer when disconnected
		node.mu.Lock()
		delete(node.Peers, actualPeerID)
		node.mu.Unlock()
	}()

	return nil
}
