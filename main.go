package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

// Handle P2P messages with professional formatting
func handleP2PMessages(node *P2PNode) {
	for msg := range node.Messages {
		timestamp := time.Now().Format("15:04:05")
		
		switch msg.Type {
		case MsgSystemStart:
			fmt.Printf("[%s] [SYSTEM] Node %s listening on port %d\n", 
				timestamp, msg.Data["nodeName"], msg.Data["port"])
				
		case MsgPeerConnected:
			direction := msg.Data["direction"].(string)
			if direction == "incoming" {
				fmt.Printf("[%s] [NETWORK] Peer %s connected from %s (%d/%d peers)\n",
					timestamp, msg.Data["peerName"], msg.Data["address"], 
					msg.Data["peerCount"], msg.Data["maxPeers"])
			} else {
				fmt.Printf("[%s] [NETWORK] Connected to peer %s at %s (%d/%d peers)\n",
					timestamp, msg.Data["peerName"], msg.Data["address"], 
					msg.Data["peerCount"], msg.Data["maxPeers"])
			}
			
		case MsgPeerDisconnected:
			fmt.Printf("[%s] [NETWORK] Peer %s disconnected\n", 
				timestamp, msg.Data["peerName"])
				
		case MsgPeersDiscovered:
			fmt.Printf("[%s] [DISCOVERY] Found %d peers via %s (total known: %d)\n",
				timestamp, msg.Data["newCount"], msg.Data["fromPeer"], msg.Data["totalKnown"])
				
		case MsgChatMessage:
			fmt.Printf("[%s] [%s] %s\n", 
				timestamp, msg.Data["from"], msg.Data["message"])
				
		case MsgBootstrapSuccess:
			fmt.Printf("[%s] [BOOTSTRAP] Connected to seed node %s\n", 
				timestamp, msg.Data["address"])
				
		case MsgBootstrapFailed:
			fmt.Printf("[%s] [BOOTSTRAP] Failed to connect to seed node %s after %d attempts\n", 
				timestamp, msg.Data["address"], msg.Data["attempts"])
				
		case MsgMaintenanceCleanup:
			fmt.Printf("[%s] [MAINTENANCE] Removed stale peer %s (%s)\n", 
				timestamp, msg.Data["peerID"], msg.Data["reason"])
				
		case MsgConnectionFailed:
			fmt.Printf("[%s] [ERROR] Failed to connect to %s: %s\n",
				timestamp, msg.Data["address"], msg.Data["error"])
				
		case MsgError:
			fmt.Printf("[%s] [ERROR] %s: %s\n", 
				timestamp, msg.Data["context"], msg.Data["error"])
				
		case MsgDebugPing:
			fmt.Printf("[%s] [DEBUG] PING -> %s\n", 
				timestamp, msg.Data["to"])
				
		case MsgDebugPong:
			fmt.Printf("[%s] [DEBUG] PONG <- %s\n", 
				timestamp, msg.Data["from"])
				
		case MsgDebugPeerRequest:
			fmt.Printf("[%s] [DEBUG] PEER_REQUEST <- %s\n", 
				timestamp, msg.Data["from"])
				
		case MsgDebugPeerResponse:
			fmt.Printf("[%s] [DEBUG] PEER_RESPONSE <- %s (%d peers)\n", 
				timestamp, msg.Data["from"], msg.Data["peerCount"])
		}
	}
}

// Print peer status in professional format
func printPeerStatus(node *P2PNode) {
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
		fmt.Println("Usage: go run . [flags] <port> [bootstrap_ip:bootstrap_port]")
		fmt.Println("\nExamples:")
		fmt.Println("  go run . 8001")
		fmt.Println("  go run . 8002 localhost:8001")
		fmt.Println("  go run . --name alice --debug 8001")
		fmt.Println("  go run . --name bob 8002 localhost:8001")
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
	
	// Seed random number generator
	rand.Seed(time.Now().UnixNano())

	node := NewP2PNode(port, bootstrapIP, bootstrapPort, *name)
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
			node.requestPeerLists()
			fmt.Println("[SYSTEM] Discovery request sent to all peers")
			continue
		}
		
		if strings.HasPrefix(input, "/connect ") {
			address := strings.TrimSpace(input[9:])
			go func() {
				err := node.ConnectToPeer(address)
				if err != nil {
					node.sendMessage(MsgConnectionFailed, map[string]interface{}{
						"address": address,
						"error":   err.Error(),
					})
				}
			}()
			continue
		}
		
		// Show our own message immediately
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] [%s] %s\n", timestamp, node.Name, input)
		
		// Broadcast message to peers
		node.BroadcastMessage(input)
	}
	
	fmt.Println("[SYSTEM] Node shutdown complete")
}
