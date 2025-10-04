package p2p

import (
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type MessageType int

const (
	MsgSystemStart MessageType = iota
	MsgPeerConnected
	MsgPeerDisconnected
	MsgPeersDiscovered
	MsgChatMessage
	MsgBytesMessage
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
	Name          string // Custom peer name (defaults to peer-<port>)
	Port          int
	Peers         map[string]*Peer
	KnownPeers    map[string]time.Time // Discovered but not necessarily connected peers
	mu            sync.RWMutex         // Single mutex for all peer data
	Listener      net.Listener
	BootstrapIP   string
	BootstrapPort int
	MaxPeers      int
	IsRunning     bool
	Messages      chan P2PMessage // Channel for UI messages
	DebugMode     bool            // Enable verbose debug output

	// Configurable timeouts
	DiscoveryInterval   time.Duration
	MaintenanceInterval time.Duration
	ConnectionTimeout   time.Duration
	PeerTimeout         time.Duration

	// Maximum allowed size for BYTES payloads; 0 means unlimited
	MaxBytesPerMessage int
}

func NewP2PNode(port int, bootstrapIP string, bootstrapPort int, name string) *P2PNode {
	// Create random ID for the node using crypto/rand
	var id string
	var randBytes [16]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		// Fallback to time-based ID if crypto/rand fails
		id = fmt.Sprintf("node-%d", time.Now().UnixNano())
	} else {
		id = fmt.Sprintf("%x", randBytes)
	}
	if name == "" {
		name = id
	}

	return &P2PNode{
		ID:                 id,
		Name:               name, // Display name
		Port:               port,
		Peers:              make(map[string]*Peer),
		KnownPeers:         make(map[string]time.Time),
		BootstrapIP:        bootstrapIP,
		BootstrapPort:      bootstrapPort,
		MaxPeers:           10,
		IsRunning:          true,
		Messages:           make(chan P2PMessage, 100), // Buffered channel
		DebugMode:          false,
		MaxBytesPerMessage: 16 * 1024 * 1024,

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
	if len(node.Peers) == 0 {
		node.mu.RUnlock()
		return
	}
	peers := make([]net.Conn, 0, len(node.Peers))
	for _, p := range node.Peers {
		if p.Conn != nil {
			peers = append(peers, p.Conn)
		}
	}
	node.mu.RUnlock()

	for _, c := range peers {
		fmt.Fprintf(c, "%s\n", message)
	}
	// Message broadcasting is silent in library mode
}

// Broadcast arbitrary bytes to all connected peers
func (node *P2PNode) BroadcastBytes(data []byte) {
	if node.MaxBytesPerMessage > 0 && len(data) > node.MaxBytesPerMessage {
		node.sendMessage(MsgError, map[string]interface{}{
			"error":   fmt.Sprintf("payload too large: %d > %d", len(data), node.MaxBytesPerMessage),
			"context": "broadcast bytes",
		})
		return
	}

	node.mu.RLock()
	if len(node.Peers) == 0 {
		node.mu.RUnlock()
		return
	}
	peers := make([]net.Conn, 0, len(node.Peers))
	for _, p := range node.Peers {
		if p.Conn != nil {
			peers = append(peers, p.Conn)
		}
	}
	node.mu.RUnlock()

	// Protocol: "BYTES:<length>\n<data>"
	header := fmt.Sprintf("BYTES:%d\n", len(data))

	for _, c := range peers {
		c.Write([]byte(header))
		c.Write(data)
	}
}

// Send bytes to a specific peer
func (node *P2PNode) SendBytesToPeer(peerID string, data []byte) error {
	node.mu.RLock()
	peer, exists := node.Peers[peerID]
	node.mu.RUnlock()

	if !exists {
		return fmt.Errorf("peer %s not found", peerID)
	}

	// Protocol: "BYTES:<length>\n<data>"
	header := fmt.Sprintf("BYTES:%d\n", len(data))

	_, err := peer.Conn.Write([]byte(header))
	if err != nil {
		return err
	}

	_, err = peer.Conn.Write(data)
	return err
}

// GetPeerInfo returns peer information for UI display
func (node *P2PNode) GetPeerInfo() (connected map[string]Peer, known map[string]time.Time, maxPeers int) {
	node.mu.RLock()
	defer node.mu.RUnlock()

	// Create copies to avoid race conditions
	connectedCopy := make(map[string]Peer)
	for id, peer := range node.Peers {
		connectedCopy[id] = *peer
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
