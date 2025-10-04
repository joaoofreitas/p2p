package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joaoofreitas/p2p"
)

// Simple custom BYTES-based file protocol frame:
// FILE:<filename>|<size>|<checksum>\n<raw file bytes>
// - <checksum>: hex-encoded SHA-256 of raw file bytes
const fileFramePrefix = "FILE:"

// sendFile reads the file, frames it into the custom BYTES protocol, and broadcasts it.
func sendFile(node *p2p.P2PNode, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("cannot read file: %w", err)
	}
	fileName := filepath.Base(filePath)
	size := len(data)

	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	header := fmt.Sprintf("FILE:%s|%d|%s\n", fileName, size, checksum)
	payload := append([]byte(header), data...)

	fmt.Printf("[SYSTEM] Starting file transfer: %s (%d bytes)\n", fileName, size)
	node.BroadcastBytes(payload)
	fmt.Printf("[SYSTEM] File transfer completed: %s\n", fileName)
	return nil
}

// handleIncomingBytes parses incoming BYTES according to the custom protocol.
// If it's not a file frame, it logs a generic binary message.
func handleIncomingBytes(from string, data []byte, timestamp string) {
	msg := string(data)
	if !strings.HasPrefix(msg, fileFramePrefix) {
		fmt.Printf("[%s] [%s] <binary data: %d bytes>\n", timestamp, from, len(data))
		return
	}

	// Split header and payload at the first newline: "FILE:<filename>|<size>|<checksum>\n<raw>"
	idx := strings.Index(msg, "\n")
	if idx <= 0 {
		fmt.Printf("[%s] [ERROR] Invalid file frame header from %s\n", timestamp, from)
		return
	}
	header := msg[:idx]
	if !strings.HasPrefix(header, fileFramePrefix) {
		fmt.Printf("[%s] [ERROR] Invalid file frame prefix from %s\n", timestamp, from)
		return
	}
	meta := strings.TrimPrefix(header, fileFramePrefix)
	fields := strings.SplitN(meta, "|", 3)
	if len(fields) != 3 {
		fmt.Printf("[%s] [ERROR] Invalid metadata in file frame from %s\n", timestamp, from)
		return
	}
	fileName := fields[0]
	sizeStr := fields[1]
	want := strings.ToLower(fields[2])
	raw := data[idx+1:]

	// raw bytes follow the header

	// Verify checksum
	sum := sha256.Sum256(raw)
	got := strings.ToLower(hex.EncodeToString(sum[:]))
	if got != want {
		fmt.Printf("[%s] [ERROR] Checksum mismatch for %s from %s (got %s, want %s)\n", timestamp, fileName, from, got, want)
		return
	}

	// Verify size (best-effort)
	if n, err := strconv.Atoi(sizeStr); err == nil && n != len(raw) {
		fmt.Printf("[%s] [WARN] Size mismatch for %s from %s (got %d, expect %d)\n", timestamp, fileName, from, len(raw), n)
	}

	// Save file
	if err := os.MkdirAll("downloads", 0o755); err != nil {
		fmt.Printf("[%s] [ERROR] Cannot create downloads directory: %v\n", timestamp, err)
		return
	}
	outPath := filepath.Join("downloads", fileName)
	// Avoid overwriting by finding next available name
	finalPath := outPath
	for i := 1; i < 10000; i++ {
		if _, err := os.Stat(finalPath); os.IsNotExist(err) {
			break
		}
		ext := filepath.Ext(fileName)
		base := strings.TrimSuffix(fileName, ext)
		finalPath = filepath.Join("downloads", fmt.Sprintf("%s-(%d)%s", base, i, ext))
	}

	if err := os.WriteFile(finalPath, raw, 0o644); err != nil {
		fmt.Printf("[%s] [ERROR] Cannot write file %s: %v\n", timestamp, finalPath, err)
		return
	}

	fmt.Printf("[%s] [FILE] %s sent file: %s (%d bytes) -> %s\n", timestamp, from, fileName, len(raw), finalPath)
}

// Handle P2P messages with professional formatting
func handleP2PMessages(node *p2p.P2PNode) {
	for msg := range node.Messages {
		timestamp := time.Now().Format("15:04:05")

		switch msg.Type {
		case p2p.MsgSystemStart:
			fmt.Printf("[%s] [SYSTEM] Node %s listening on port %d\n",
				timestamp, msg.Data["nodeName"], msg.Data["port"])

		case p2p.MsgPeerConnected:
			direction := msg.Data["direction"].(string)
			if direction == "incoming" {
				fmt.Printf("[%s] [NETWORK] Peer [%s] connected from %s (%d/%d peers)\n",
					timestamp, msg.Data["peerName"], msg.Data["address"],
					msg.Data["peerCount"], msg.Data["maxPeers"])
			} else {
				fmt.Printf("[%s] [NETWORK] Connected to peer [%s] at %s (%d/%d peers)\n",
					timestamp, msg.Data["peerName"], msg.Data["address"],
					msg.Data["peerCount"], msg.Data["maxPeers"])
			}

		case p2p.MsgPeerDisconnected:
			fmt.Printf("[%s] [NETWORK] Peer [%s] disconnected\n",
				timestamp, msg.Data["peerName"])

		case p2p.MsgPeersDiscovered:
			fmt.Printf("[%s] [DISCOVERY] Found %d peers via %s (total known: %d)\n",
				timestamp, msg.Data["newCount"], msg.Data["fromPeer"], msg.Data["totalKnown"])

		case p2p.MsgChatMessage:
			fromName, _ := msg.Data["from"].(string)
			fmt.Printf("[%s] [%s] %s\n",
				timestamp, fromName, msg.Data["message"])

		case p2p.MsgBytesMessage:
			data := msg.Data["data"].([]byte)
			from := msg.Data["from"].(string)
			handleIncomingBytes(from, data, timestamp)

		case p2p.MsgBootstrapSuccess:
			fmt.Printf("[%s] [BOOTSTRAP] Connected to seed node %s\n",
				timestamp, msg.Data["address"])

		case p2p.MsgBootstrapFailed:
			fmt.Printf("[%s] [BOOTSTRAP] Failed to connect to seed node %s after %d attempts\n",
				timestamp, msg.Data["address"], msg.Data["attempts"])

		case p2p.MsgMaintenanceCleanup:
			fmt.Printf("[%s] [MAINTENANCE] Removed stale peer %s (%s)\n",
				timestamp, msg.Data["peerID"], msg.Data["reason"])

		case p2p.MsgConnectionFailed:
			fmt.Printf("[%s] [ERROR] Failed to connect to %s: %s\n",
				timestamp, msg.Data["address"], msg.Data["error"])

		case p2p.MsgError:
			fmt.Printf("[%s] [ERROR] %s: %s\n",
				timestamp, msg.Data["context"], msg.Data["error"])

		case p2p.MsgDebugPing:
			fmt.Printf("[%s] [DEBUG] PING -> %s\n",
				timestamp, msg.Data["to"])

		case p2p.MsgDebugPong:
			fmt.Printf("[%s] [DEBUG] PONG <- %s\n",
				timestamp, msg.Data["from"])

		case p2p.MsgDebugPeerRequest:
			fmt.Printf("[%s] [DEBUG] PEER_REQUEST <- %s\n",
				timestamp, msg.Data["from"])

		case p2p.MsgDebugPeerResponse:
			fmt.Printf("[%s] [DEBUG] PEER_RESPONSE <- %s (%d peers)\n",
				timestamp, msg.Data["from"], msg.Data["peerCount"])
		}
	}
}

// Print peer status in professional format
func printPeerStatus(node *p2p.P2PNode) {
	connected, known, maxPeers := node.GetPeerInfo()

	fmt.Printf("\n[STATUS] Connected Peers (%d/%d):\n", len(connected), maxPeers)
	if len(connected) == 0 {
		fmt.Printf("  No active connections\n")
	} else {
		for _, peer := range connected {
			fmt.Printf("  %s (%s) - last seen: %s\n",
				peer.Name, peer.Address, peer.LastSeen.Format("15:04:05"))
		}
	}

	fmt.Printf("\n[STATUS] Known Peers (%d):\n", len(known))
	if len(known) == 0 {
		fmt.Printf("  No discovered peers\n")
	} else {
		count := 0
		for address, discovered := range known {
			if count >= 10 { // Limit display
				fmt.Printf("  ... and %d more\n", len(known)-10)
				break
			}

			// Check if connected
			status := "discovered"
			for _, peer := range connected {
				if peer.Address == address {
					status = "connected"
					break
				}
			}

			fmt.Printf("  %s (%s) - %s\n", address, status, discovered.Format("15:04:05"))
			count++
		}
	}
	fmt.Println()
}

func main() {
	// Define flags
	var debug = flag.Bool("debug", false, "Enable debug output (pings, peer requests)")
	var name = flag.String("name", "", "Custom peer name (defaults to random id)")

	flag.Usage = func() {
		fmt.Println("Usage: go run ./cmd/p2p [flags] <port> [bootstrap_ip:bootstrap_port]")
		fmt.Println("\nExamples:")
		fmt.Println("  go run ./cmd/p2p 8001")
		fmt.Println("  go run ./cmd/p2p 8002 localhost:8001")
		fmt.Println("  go run ./cmd/p2p --name alice --debug 8001")
		fmt.Println("  go run ./cmd/p2p --name bob 8002 localhost:8001")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()
	}

	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	port, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Printf("Invalid port: %v\n", err)
		os.Exit(1)
	}

	var bootstrapIP string
	var bootstrapPort int

	if len(args) > 1 {
		parts := strings.Split(args[1], ":")
		if len(parts) == 2 {
			bootstrapIP = parts[0]
			bootstrapPort, err = strconv.Atoi(parts[1])
			if err != nil {
				fmt.Printf("Invalid bootstrap port: %v\n", err)
				os.Exit(1)
			}
		}
	}

	node := p2p.NewP2PNode(port, bootstrapIP, bootstrapPort, *name)
	node.DebugMode = *debug

	// Start the server
	err = node.StartServer()
	if err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		os.Exit(1)
	}
	defer node.Shutdown()

	// Start discovery and maintenance
	node.StartDiscoveryAndMaintenance()

	// Start message handler for professional output
	go handleP2PMessages(node)

	// Bootstrap if needed
	go node.Bootstrap()

	// Command line interface
	fmt.Println("\n[COMMANDS]")
	fmt.Println("  /peers - Display network status")
	fmt.Println("  /connect <ip:port> - Establish peer connection")
	fmt.Println("  /discover - Request peer discovery")
	fmt.Println("  /send <filepath> - Send file to all peers")
	fmt.Println("  /quit - Shutdown node")
	fmt.Println("  Type message to broadcast to network")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			continue
		}

		if input == "/quit" {
			break
		}

		if input == "/peers" {
			printPeerStatus(node)
			continue
		}

		if input == "/discover" {
			node.BroadcastMessage("REQUEST_PEERS")
			fmt.Println("[SYSTEM] Discovery request sent to all peers")
			continue
		}

		if strings.HasPrefix(input, "/connect ") {
			address := strings.TrimSpace(input[9:])
			go func() {
				err := node.ConnectToPeer(address)
				if err != nil {
					fmt.Printf("[ERROR] Failed to connect to %s: %v\n", address, err)
				}
			}()
			continue
		}

		if strings.HasPrefix(input, "/send ") {
			filePath := strings.TrimSpace(input[6:])
			if filePath == "" {
				fmt.Println("[ERROR] Please specify a file path")
				continue
			}

			// Check if file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				fmt.Printf("[ERROR] File not found: %s\n", filePath)
				continue
			}

			go func() {
				if err := sendFile(node, filePath); err != nil {
					fmt.Printf("[ERROR] Failed to send file: %v\n", err)
				}
			}()
			continue
		}

		// Show our own message immediately (with our name in brackets)
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] [%s] %s\n", timestamp, node.Name, input)

		// Broadcast message to peers
		node.BroadcastMessage(input)
	}

	fmt.Println("[SYSTEM] Node shutdown complete")
}
