# P2P Network Library

A simple, modular peer-to-peer message networking library written in Go.

## ⚠️ Important Security Notice

**This is a proof of concept implementation and should NOT be used in production environments.**

- **No encryption** - All data is sent in plaintext over TCP connections
- **No authentication** - Anyone can connect and impersonate peers
- **No data integrity checks** - File transfers may be corrupted without detection
- **No access control** - Any peer can send files to any other peer

This library is intended for educational purposes, local development, and trusted network environments only.

## Features

- **Full mesh network** - All peers connect to all other peers
- **Immediate peer discovery on connection** - No waiting for periodic timers
- **Modular design** - Library can be embedded in other applications
- **Event driven architecture** - Message channel for handling events
- **File sharing** - Send arbitrary binary data and files between peers

## Quick Start

The `cmd/p2p/main.go` file provides a simple CLI interface for testing and demonstration that includes both chat and file sharing capabilities. This can be adapted to other use cases by implementing custom message parsing and handling logic.

The library exposes a generic BYTES channel via `MsgBytesMessage` for arbitrary binary protocols (HTTP-like, FTP-like, or fully custom). The CLI demonstrates one such protocol by implementing a simple chunked file transfer on top of BYTES.

### Basic Usage

In the future, we might use DHT or other discovery methods, but for now, you need to manually connect nodes.

```bash
# Start first node
go run ./cmd/p2p 8001

# Start second node that connects to first
go run ./cmd/p2p 8002 localhost:8001

Send a file from first node
# In first terminal: /send test.txt

# Start third node that connects to second
go run ./cmd/p2p 8003 localhost:8002
```

### With Custom Name
```bash
go run ./cmd/p2p --name alice 8001
go run ./cmd/p2p --name bob 8002 localhost:8001
```

### Debug Mode
```bash
go run ./cmd/p2p --debug 8001  # Shows ping/pong messages and verbose output
```

## Library Usage

### Basic Setup

```go
package main

import (
    "fmt"
    "time"

    "github.com/joaoofreitas/p2p"
)

func main() {
    // Create node
    node := p2p.NewP2PNode(8001, "", 0, "")

    // Optional: limit BYTES payload size (default 16 MiB)
    node.MaxBytesPerMessage = 16 * 1024 * 1024

    // BYTES handling: implement your own protocol in MsgBytesMessage case below.
    // For example, you can stream chunked file data, RPC frames, etc.
    // This README no longer uses a built-in file receiver in the library.
    // See cmd/p2p/main.go for a complete chunked file transfer example.
    //
    // The library only transports bytes; parsing is up to your application.
    //
    // Tip: keep messages small or chunk large payloads for reliability.
    //
    // Downloads or persistence are application responsibilities.
    //

    // Handle messages
    go func() {
        for msg := range node.Messages {
            switch msg.Type {
            case p2p.MsgPeerConnected:
                fmt.Printf("New peer: %s\n", msg.Data["peerID"])
            case p2p.MsgChatMessage:
                fmt.Printf("[%s]: %s\n", msg.Data["from"], msg.Data["message"])
            case p2p.MsgBytesMessage:
                data := msg.Data["data"].([]byte)
                from := msg.Data["from"].(string)
                fmt.Printf("[BYTES] from %s (%d bytes)\n", from, len(data))
            }
            // Parse your custom protocol here
        }
    }()

    // Start server
    _ = node.StartServer()
    node.StartDiscoveryAndMaintenance()

    // Connect to bootstrap peer
    if err := node.ConnectToPeer("localhost:8000"); err != nil {
        fmt.Printf("Bootstrap failed: %v\n", err)
    }

    // Send message
    node.BroadcastMessage("Hello network!")

    // Send binary data using your custom protocol
    node.BroadcastBytes([]byte("custom protocol data"))

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
- `MsgBytesMessage` - Received binary data from peer

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
- `cmd/p2p/main.go` - CLI interface and message formatting
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
- `/send <filepath>` - Send file to all peers
- `/quit` - Shutdown node
- `<message>` - Broadcast message to all peers

### Handshake Example

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

### Binary Messages
```
BYTES:<length>          # Binary data header
<binary_data>           # Raw binary data
```
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
node := NewP2PNode(8001, "bootstrap.example.com", 8000, "")
node.DiscoveryInterval = 30 * time.Second    // Peer discovery frequency
node.MaintenanceInterval = 15 * time.Second  // Connection cleanup frequency
node.ConnectionTimeout = 10 * time.Second    // TCP connection timeout
node.PeerTimeout = 60 * time.Second          // Consider peer dead after
node.MaxPeers = 20                           // Maximum concurrent connections
node.MaxBytesPerMessage = 16 * 1024 * 1024   // Limit BYTES payloads (optional)
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

## Example: File Transfer over BYTES (CLI)

The library transports raw bytes via `MsgBytesMessage`. The CLI demonstrates a minimal chunked file transfer protocol composed of three frame types:

FILESTART:<filename>|<size>|<checksum>

<raw file bytes>
FILEEND:<filename>|<checksum>
```

- size: total byte length of the file
- chunks: sent as separate BYTES frames; each `FILECHUNK` header line is followed by raw bytes
High-level flow:

- Receiver: opens downloads/<name>.part, appends chunk bytes, updates a running SHA-256, verifies size and checksum at `FILEEND`, and renames to the final path with non-overwriting semantics.
On the wire (conceptually):
```
BYTES:<len>\nFILESTART:document.pdf|1024|<hex_sha256>

BYTES:<len>\nFILEEND:document.pdf|<hex_sha256>
```
CLI usage:
- `/send <filepath>` broadcasts a file to all connected peers.

Build your own protocols:
- Define your own framing and semantics (e.g., token-delimited strings, JSON, protobuf, msgpack).
- Serialize to bytes and send using `BroadcastBytes` or `SendBytesToPeer`.
- In your application, handle `MsgBytesMessage` to parse incoming payloads and implement your logic.

## Example Output

```
[15:04:32] [SYSTEM] Node alice listening on port 8001
[15:04:35] [NETWORK] Peer bob connected from 192.168.1.100:8002 (1/10 peers)
[15:04:36] [DISCOVERY] Found 3 peers via bob (total known: 5)
[15:04:36] [NETWORK] Connected to peer charlie at localhost:8003 (2/10 peers)
[15:04:40] [bob] network status looks good
[15:04:42] [charlie] all systems operational
[15:04:43] [SYSTEM] Starting file transfer: test.txt (342 bytes)
[15:04:43] [FILE] bob is sending document.pdf (1024 bytes)
[15:04:44] [FILE] document.pdf progress: 100.0% (1024/1024 bytes)
[15:04:44] [FILE] bob sent file: document.pdf (1024 bytes) -> downloads/document.pdf
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
