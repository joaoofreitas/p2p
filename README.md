# P2P Network Library

A simple, modular peer-to-peer message networking library written in Go.

## Features

- **Full mesh network** - All peers connect to all other peers
- **Immediate peer discovery** - No waiting for periodic timers
- **Professional logging** - Clean, timestamped output with categories
- **Modular design** - Library can be embedded in other applications
- **Thread-safe** - Uses proper mutex locking
- **Configurable timeouts** - All intervals can be customized

## Quick Start

### Basic Usage

```bash
# Start first node
go run . 8001

# Start second node that connects to first
go run . 8002 localhost:8001

# Start third node that connects to second
go run . 8003 localhost:8002
```

### With Custom Name
```bash
go run . 8001 --name alice
go run . 8002 localhost:8001 --name bob
```

### Debug Mode
```bash
go run . 8001 --debug  # Shows ping/pong messages and verbose output
```

## Library Usage

### Basic Setup

```go
package main

import (
    "fmt"
    "time"
)

func main() {
    // Create node
    node := NewP2PNode(8001, "", 0)
    
    // Handle messages
    go func() {
        for msg := range node.Messages {
            switch msg.Type {
            case MsgPeerConnected:
                fmt.Printf("New peer: %s\n", msg.Data["peerID"])
            case MsgChatMessage:
                fmt.Printf("[%s]: %s\n", msg.Data["from"], msg.Data["message"])
            }
        }
    }()
    
    // Start server
    node.StartServer()
    node.StartDiscoveryAndMaintenance()
    
    // Connect to bootstrap peer
    if err := node.ConnectToPeer("localhost:8000"); err != nil {
        fmt.Printf("Bootstrap failed: %v\n", err)
    }
    
    // Send message
    node.BroadcastMessage("Hello network!")
    
    // Keep running
    select {}
}
```

## Message Types

The library communicates through a message channel with these types:

### System Messages
- `MsgSystemStart` - Node started listening
- `MsgError` - Error occurred
- `MsgBootstrapSuccess/Failed` - Bootstrap attempt results

### Network Messages  
- `MsgPeerConnected` - New peer joined network
- `MsgPeerDisconnected` - Peer left network
- `MsgPeersDiscovered` - Found new peers through discovery

### Chat Messages
- `MsgChatMessage` - Received chat message from peer

### Maintenance Messages
- `MsgMaintenanceCleanup` - Removed stale peer
- `MsgConnectionFailed` - Failed to connect to peer

### Message Structure

```go
type P2PMessage struct {
    Type MessageType
    Data map[string]interface{}
}
```

## Architecture

### File Structure
- `main.go` - CLI interface and message formatting
- `node.go` - Core P2PNode struct and configuration  
- `connection.go` - Connection handling and peer management
- `discovery.go` - Peer discovery and network maintenance

### Network Topology

This implements a **full mesh network** where every peer connects to every other peer:

```
    Alice ←→ Bob
      ↑       ↑
      ↓       ↓
   Charlie ←→ Dave
```

## CLI Commands

When running the CLI interface:

- `/peers` - Display network status
- `/connect <ip:port>` - Establish peer connection
- `/discover` - Request peer discovery
- `/quit` - Shutdown node
- `<message>` - Broadcast message to all peers

## Network Protocol Commands

These are the actual commands sent over TCP connections between peers:

### Handshake Protocol
```
Client -> Server: ID:NAME
Server -> Client: ID:NAME or DUPLICATE or MAX_PEERS
```

**Examples:**
- `peer-8001:alice` - Peer with ID "peer-8001" and name "alice"
- `peer-8002:peer-8002` - Peer with no custom name (defaults to ID)
- `DUPLICATE` - Server rejects connection (already connected)
- `MAX_PEERS` - Server rejects connection (too many peers)

### Maintenance Commands
```
PING                    # Health check
PONG                    # Response to PING
REQUEST_PEERS           # Request peer list
PEERS:<addr1>,<addr2>   # Peer list response
```

**Examples:**
- `PING` - Send heartbeat
- `PONG` - Acknowledge heartbeat
- `REQUEST_PEERS` - Ask for known peer addresses
- `PEERS:localhost:8001,localhost:8003,192.168.1.100:8004` - Share 3 peer addresses

### Chat Messages
```
<any_text>              # Broadcast message
```

**Examples:**
- `hello everyone!` - Simple chat message
- `network status check` - Status inquiry
- `shutting down in 5 minutes` - Notification

### Connection Flow
```
1. TCP Connect to peer
2. Send: ID:NAME
3. Receive: ID:NAME (success) or DUPLICATE/MAX_PEERS (rejection)
4. Send: REQUEST_PEERS (immediate discovery)
5. Receive: PEERS:<addr1>,<addr2>,...
6. Send: PEERS:<my_known_peers> (share back)
7. Normal operation: PING/PONG every 15s, chat messages
```

## Configuration

### Timeouts (configurable in NewP2PNode):

```go
node := NewP2PNode(8001, "bootstrap.example.com", 8000)
node.DiscoveryInterval = 30 * time.Second    // Peer discovery frequency
node.MaintenanceInterval = 15 * time.Second  // Connection cleanup frequency
node.ConnectionTimeout = 10 * time.Second    // TCP connection timeout
node.PeerTimeout = 60 * time.Second          // Consider peer dead after
node.MaxPeers = 20                           // Maximum concurrent connections
```

## Limitations

### Scalability
- **Not suitable for large networks** - Full mesh means N*(N-1)/2 total connections
- **Memory usage grows O(N)** - Each node stores info about all other nodes
- **Network chatter increases O(N²)** - Every message broadcast to all peers

### Recommended Limits
- **< 50 peers** for good performance
- **< 100 peers** maximum practical limit

### Use Cases
- **Small team networks** (5-20 people)
- **Local area networks**
- **Development/testing environments**
- **IoT device coordination** (small clusters)

### What This Is NOT
- Not a DHT (Distributed Hash Table)
- Not suitable for public networks like BitTorrent
- Not Byzantine fault tolerant
- No built-in encryption/authentication

## Example Output

```
[15:04:32] [SYSTEM] Node alice listening on port 8001
[15:04:35] [NETWORK] Peer bob connected from 192.168.1.100:8002 (1/10 peers)
[15:04:36] [DISCOVERY] Found 3 peers via bob (total known: 5)
[15:04:36] [NETWORK] Connected to peer charlie at localhost:8003 (2/10 peers)
[15:04:40] [bob] network status looks good
[15:04:42] [charlie] all systems operational
[15:04:45] [MAINTENANCE] Removed stale peer dave (timeout)
```

## Debug Output

With `--debug` flag:

```
[15:04:50] [DEBUG] Sent PING to bob
[15:04:50] [DEBUG] Received PONG from bob
[15:04:50] [DEBUG] Sent REQUEST_PEERS to charlie
[15:04:51] [DEBUG] Received PEERS: localhost:8004,localhost:8005 from charlie
```

## Contributing

This is a simple educational P2P library. Feel free to extend it with:

- NAT traversal
- Encryption/authentication
- Better network topologies (DHT, gossip protocols)
- Message routing/relay capabilities
- Persistence and network recovery
