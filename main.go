package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type FileTransferMessage struct {
	Type     string `json:"type"`     // "file_start", "file_chunk", "file_end"
	FileName string `json:"filename"`
	FileSize int64  `json:"filesize"`
	ChunkID  int    `json:"chunk_id"`
	Data     []byte `json:"data"`
}

var activeFileTransfers = make(map[string]*FileTransfer)

type FileTransfer struct {
	FileName     string
	FileSize     int64
	ReceivedSize int64
	File         *os.File
	Chunks       map[int][]byte
}

// Send a file to all peers
func sendFile(node *P2PNode, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %v", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("cannot get file info: %v", err)
	}

	fileName := filepath.Base(filePath)
	fileSize := fileInfo.Size()

	// Send file start message
	startMsg := FileTransferMessage{
		Type:     "file_start",
		FileName: fileName,
		FileSize: fileSize,
	}

	data, _ := json.Marshal(startMsg)
	node.BroadcastBytes(data)

	fmt.Printf("[SYSTEM] Starting file transfer: %s (%d bytes)\n", fileName, fileSize)

	// Send file in chunks
	const chunkSize = 32768 // 32KB chunks
	buffer := make([]byte, chunkSize)
	chunkID := 0

	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading file: %v", err)
		}

		chunkMsg := FileTransferMessage{
			Type:     "file_chunk",
			FileName: fileName,
			ChunkID:  chunkID,
			Data:     buffer[:n],
		}

		data, _ := json.Marshal(chunkMsg)
		node.BroadcastBytes(data)

		chunkID++
	}

	// Send file end message
	endMsg := FileTransferMessage{
		Type:     "file_end",
		FileName: fileName,
	}

	data, _ = json.Marshal(endMsg)
	node.BroadcastBytes(data)

	fmt.Printf("[SYSTEM] File transfer completed: %s\n", fileName)
	return nil
}

// Handle incoming file transfer messages
func handleFileTransfer(msg FileTransferMessage, from string, timestamp string) {
	transferKey := from + ":" + msg.FileName

	switch msg.Type {
	case "file_start":
		fmt.Printf("[%s] [FILE] %s is sending file: %s (%d bytes)\n",
			timestamp, from, msg.FileName, msg.FileSize)

		// Create downloads directory if it doesn't exist
		os.MkdirAll("downloads", 0755)

		// Create file
		filePath := filepath.Join("downloads", msg.FileName)
		file, err := os.Create(filePath)
		if err != nil {
			fmt.Printf("[ERROR] Cannot create file %s: %v\n", filePath, err)
			return
		}

		activeFileTransfers[transferKey] = &FileTransfer{
			FileName:     msg.FileName,
			FileSize:     msg.FileSize,
			ReceivedSize: 0,
			File:         file,
			Chunks:       make(map[int][]byte),
		}

	case "file_chunk":
		transfer, exists := activeFileTransfers[transferKey]
		if !exists {
			fmt.Printf("[ERROR] Received chunk for unknown file: %s\n", msg.FileName)
			return
		}

		// Store chunk
		transfer.Chunks[msg.ChunkID] = msg.Data
		transfer.ReceivedSize += int64(len(msg.Data))

		// Write chunks in order
		for i := 0; ; i++ {
			if chunk, exists := transfer.Chunks[i]; exists {
				transfer.File.Write(chunk)
				delete(transfer.Chunks, i)
			} else {
				break
			}
		}

		// Show progress
		progress := float64(transfer.ReceivedSize) / float64(transfer.FileSize) * 100
		fmt.Printf("[%s] [FILE] Progress: %s - %.1f%% (%d/%d bytes)\n",
			timestamp, msg.FileName, progress, transfer.ReceivedSize, transfer.FileSize)

	case "file_end":
		transfer, exists := activeFileTransfers[transferKey]
		if !exists {
			fmt.Printf("[ERROR] Received end for unknown file: %s\n", msg.FileName)
			return
		}

		// Write any remaining chunks
		for i := 0; i < len(transfer.Chunks)+10; i++ {
			if chunk, exists := transfer.Chunks[i]; exists {
				transfer.File.Write(chunk)
				delete(transfer.Chunks, i)
			}
		}

		transfer.File.Close()
		delete(activeFileTransfers, transferKey)

		fmt.Printf("[%s] [FILE] Completed: %s saved to downloads/%s\n",
			timestamp, msg.FileName, msg.FileName)
	}
}

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

		case MsgBytesMessage:
			data := msg.Data["data"].([]byte)
			from := msg.Data["from"].(string)
			length := msg.Data["length"].(int)

			// Try to parse as file transfer JSON
			var fileMsg FileTransferMessage
			if err := json.Unmarshal(data, &fileMsg); err == nil {
				handleFileTransfer(fileMsg, from, timestamp)
			} else {
				fmt.Printf("[%s] [%s] <binary data: %d bytes>\n",
					timestamp, from, length)
			}
				
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
				err := sendFile(node, filePath)
				if err != nil {
					fmt.Printf("[ERROR] Failed to send file: %v\n", err)
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
