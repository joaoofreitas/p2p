package main

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
    "math/rand"
)

type MessageType int

const (
	MsgSystemStart MessageType = iota
	MsgPeerConnected
	MsgPeerDisconnected
	MsgPeersDiscovered
	MsgChatMessage
	MsgBootstrapSuccess
	MsgBootstrapFailed
	MsgConnectionFailed
	MsgMaintenanceCleanup
	MsgError
	MsgDebugPing
	MsgDebugPong
	MsgDebugPeerRequest
	MsgDebugPeerResponse
)

type P2PMessage struct {
	Type MessageType
	Data map[string]interface{}
}

type Peer struct {
	ID       string
	Name     string
	Address  string
	Conn     net.Conn
	LastSeen time.Time
}

type P2PNode struct {
	ID            string
	Name          string               // Custom peer name (defaults to peer-<port>)
	Port          int
	Peers         map[string]*Peer
	KnownPeers    map[string]time.Time // Discovered but not necessarily connected peers
	mu            sync.RWMutex         // Single mutex for all peer data
	Listener      net.Listener
	BootstrapIP   string
	BootstrapPort int
	MaxPeers      int
	IsRunning     bool
	Messages      chan P2PMessage      // Channel for UI messages
	DebugMode     bool                 // Enable verbose debug output

	// Configurable timeouts
	DiscoveryInterval time.Duration
	MaintenanceInterval time.Duration
	ConnectionTimeout time.Duration
	PeerTimeout time.Duration
}

func NewP2PNode(port int, bootstrapIP string, bootstrapPort int, name string) *P2PNode {
    // Create random uuid for the node
    id := fmt.Sprintf("%x-%x-%x-%x-%x", rand.Int31(), rand.Int31(), rand.Int31(), rand.Int31(), rand.Int31())
    if name == "" {
        name = id
    }

	return &P2PNode{
		ID:            id,
		Name:          name,               // Display name
		Port:          port,
		Peers:         make(map[string]*Peer),
		KnownPeers:    make(map[string]time.Time),
		BootstrapIP:   bootstrapIP,
		BootstrapPort: bootstrapPort,
		MaxPeers:      10,
		IsRunning:     true,
		Messages:      make(chan P2PMessage, 100), // Buffered channel
		DebugMode:     false,

		// Default timeouts
		DiscoveryInterval:   30 * time.Second,
		MaintenanceInterval: 15 * time.Second,
		ConnectionTimeout:   10 * time.Second,
		PeerTimeout:         60 * time.Second,
	}
}

// Helper function to send messages to UI
func (node *P2PNode) sendMessage(msgType MessageType, data map[string]interface{}) {
	select {
	case node.Messages <- P2PMessage{Type: msgType, Data: data}:
	default:
		// Channel full, drop message to prevent blocking
	}
}

// Send debug messages only if debug mode is enabled
func (node *P2PNode) sendDebugMessage(msgType MessageType, data map[string]interface{}) {
	if node.DebugMode {
		node.sendMessage(msgType, data)
	}
}

// Check if an address belongs to this node
func (node *P2PNode) isMyAddress(address string) bool {
	return strings.HasSuffix(address, fmt.Sprintf(":%d", node.Port))
}

// Broadcast message to all connected peers
func (node *P2PNode) BroadcastMessage(message string) {
	node.mu.RLock()
	defer node.mu.RUnlock()

	if len(node.Peers) == 0 {
		return
	}

	for _, peer := range node.Peers {
		fmt.Fprintf(peer.Conn, "%s\n", message)
	}
	// Message broadcasting is silent in library mode
}

// GetPeerInfo returns peer information for UI display
func (node *P2PNode) GetPeerInfo() (connected map[string]*Peer, known map[string]time.Time, maxPeers int) {
	node.mu.RLock()
	defer node.mu.RUnlock()

	// Create copies to avoid race conditions
	connectedCopy := make(map[string]*Peer)
	for id, peer := range node.Peers {
		connectedCopy[id] = peer
	}

	knownCopy := make(map[string]time.Time)
	for addr, time := range node.KnownPeers {
		knownCopy[addr] = time
	}

	return connectedCopy, knownCopy, node.MaxPeers
}

func (node *P2PNode) Shutdown() {
	node.IsRunning = false

	if node.Listener != nil {
		node.Listener.Close()
	}

	node.mu.Lock()
	for _, peer := range node.Peers {
		peer.Conn.Close()
	}
	node.mu.Unlock()
}
