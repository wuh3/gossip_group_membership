package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mp2/utils"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

var exit = false
var debug = false
var INTRODUCER_ID = utils.INTRODUCER_IP + ":" + utils.INTRODUCER_PORT
var LOCAL_IP = utils.INTRODUCER_IP
var LOCAL_ID = utils.GenerateNodeId()
var LOCAL_LIST utils.MembershipList
var LIST_TO_SEND utils.MembershipList
var SUSPICION = "OFF"

var HeartbeatChannel chan utils.HeartBeat
var StopChannel chan struct{}

func listenUserInputIntroducer() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Enter commands Enter commands from this list: join, list_mem, list_self, leave, enable suspicion, disable suspicion")

	for {
		fmt.Print("> ")
		userInput, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input:", err)
			return
		}

		// Remove newline character from the input
		userInput = strings.TrimSpace(userInput)

		// Process the user's input
		switch userInput {

		case "list_mem":
			listMembership()
		case "list_self":
			listSelfId()
		//case "leave":
		//	leave()
		case "enable suspicion":
			SUSPICION = "ON"
			utils.Tclear = 2000
		case "disable suspicion":
			SUSPICION = "OFF"
			utils.Tclear = 0
		case "exit":
			exit = true
		default:
			fmt.Printf("Wrong cmd entered: %s\n", userInput)
			fmt.Println("Please input command from this list: join, list_mem, list_self, leave, enable suspicion, disable suspicion")
		}

	}
}

func listSelfId() {
	member, ok := LOCAL_LIST[LOCAL_ID]
	if ok {
		fmt.Println("ID IPaddress TimeStamp Status")
		fmt.Print(member.Id + " ")
		fmt.Print(member.IPAddress + " ")
		fmt.Print(member.Version + " ")
		fmt.Print(member.Status)
		fmt.Println()
	}
}

func listMembership() {
	fmt.Println("ID IPaddress TimeStamp Status")
	for _, member := range LIST_TO_SEND {
		_, ok := LIST_TO_SEND[member.Id]
		if ok {
			fmt.Print(member.Id + " ")
			fmt.Print(member.IPAddress + " ")
			fmt.Print(member.Version + " ")
			fmt.Print(member.Status)
			fmt.Println("\n----------")
		}
	}
}

func handleHeartBeat(stopChan chan struct{}) {
	fmt.Println("enter heartbeat handler")

	receiverAddress := LOCAL_IP + ":" + utils.PORT
	udpAddr, err := net.ResolveUDPAddr("udp", receiverAddress)
	if err != nil {
		fmt.Println("Error resolving address:", err)
		return
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println("Error creating UDP connection:", err)
		return
	}
	defer conn.Close()

	buffer := make([]byte, 4096)

	for {
		if debug {
			fmt.Println("enter heartbeat handler loop")
		}

		select {
		case <-stopChan:
			fmt.Println("Stopping heartbeat handler...")
			conn.Close()
			return
		default:
			n, _, err := conn.ReadFromUDP(buffer)
			if debug {
				fmt.Println(n)
			}
			if err != nil {
				fmt.Println("Error reading from UDP connection:", err)
				continue
			}

			if n != 0 {
				var receivedHB utils.HeartBeat
				err := json.Unmarshal(buffer[:n], &receivedHB)
				if err != nil {
					fmt.Println("Error unmarshaling heartbeat:", err)
					continue
				}
				SUSPICION = receivedHB.Suspicion
				if strings.Compare(SUSPICION, "ON") == 0 {
					utils.Tclear = 2000
				} else if strings.Compare(SUSPICION, "OFF") == 0 {
					utils.Tclear = 0
				}

				if debug {
					fmt.Println("forwarding heartbeat")
				}

				// Update the local membership list
				receivedList := receivedHB.List
				updateList(receivedList)

				if debug {
					fmt.Println("printing received heartbeat")
					for _, member := range receivedList {
						_, ok := receivedList[member.Id]
						if ok {
							fmt.Println("ID IPaddress TimeStamp Status")
							fmt.Print(member.Id + " ")
							fmt.Print(member.IPAddress + " ")
							fmt.Print(member.Version + " ")
							fmt.Print(member.Status)
							fmt.Println()
						}
					}

				}
			}
		}
	}
	fmt.Println("exit heartbeat handler")
}

func updateList(list utils.MembershipList) {
	if len(LOCAL_LIST) == 5 || len(LOCAL_LIST) == 10 {
		listMembership()
	}
	for _, member := range list {
		_, ok := LOCAL_LIST[member.Id]
		if ok {
			local_time_str := LOCAL_LIST[member.Id].Version
			local_time, _ := time.Parse(utils.LAYOUT_TIME, local_time_str)
			new_time, _ := time.Parse(utils.LAYOUT_TIME, member.Version)
			if new_time.After(local_time) || member.Status == "FAILED" {
				LOCAL_LIST[member.Id] = member
			} else if new_time.Equal(local_time) {
				if member.Status == "SUSPECT" {
					// suspect > alive
					LOCAL_LIST[member.Id] = member
				}
			}
		} else {
			// new node
			if member.Status != "FAILED" {
				LOCAL_LIST[member.Id] = member
			}
		}
	}
	LIST_TO_SEND = LOCAL_LIST
}

func deleteFailedNode() {
	if debug {
		fmt.Println("enter deleteFailedNode()")
	}
	for _, member := range LOCAL_LIST {
		if member.Status == "FAILED" && strings.Compare(member.Id, LOCAL_ID) != 0 {
			if debug {
				fmt.Println("deleting: ", member.Id)
			}
			delete(LOCAL_LIST, member.Id)
		}
	}
	if debug {
		fmt.Println("exit deleteFailedNode()")
	}

}

/*
*
TODO:
 1. Constantly monitor the local membership list, mark suspect and failed
 2. No deletion performed here
*/
func monitorList() {
	if debug {
		fmt.Println("enter monitorList")
	}
	var wg sync.WaitGroup
	for _, member := range LOCAL_LIST {
		wg.Add(1)
		if strings.Compare(member.Id, LOCAL_ID) == 0 { // LOCAL_ID is ID of introducer
			member.Status = "OK"
			member.Version = time.Now().Format(utils.LAYOUT_TIME)
			LOCAL_LIST[member.Id] = member
			continue
		}

		go func(m *utils.Member, group *sync.WaitGroup) {
			defer group.Done()
			mTime, _ := time.Parse(utils.LAYOUT_TIME, m.Version)
			localClock, _ := time.Parse(utils.LAYOUT_TIME, time.Now().Format(utils.LAYOUT_TIME))
			diff := localClock.Sub(mTime)

			if strings.Compare(SUSPICION, "ON") == 0 {
				if m.Status == "OK" && diff > 4000*time.Millisecond {
					if debug {
						fmt.Println("time after suspectTime")
					}
					// If suspicion enabled
					m.Status = "SUSPECT"
					m.Version = time.Now().Format(utils.LAYOUT_TIME)
					LOCAL_LIST[m.Id] = *m
					fmt.Println("Node " + m.Id + " is suspected!")
					listMembership()
				} else if m.Status == "SUSPECT" && diff > 2000*time.Millisecond {
					m.Status = "FAILED"
					LOCAL_LIST[m.Id] = *m
					fmt.Println("Node " + m.Id + " is failed!")
					listMembership()
					if debug {
						fmt.Println("Failed: ", m.Id)
					}

				}
			} else if strings.Compare(SUSPICION, "OFF") == 0 {
				if m.Status == "OK" && diff > 4000*time.Millisecond {
					m.Status = "FAILED"
					fmt.Println("Node " + m.Id + " is failed!")
					listMembership()
					LOCAL_LIST[m.Id] = *m
					if debug {
						fmt.Println("Failed: ", m.Id)
					}
				}
			}
		}(&member, &wg)
	}
	LIST_TO_SEND = LOCAL_LIST
	deleteFailedNode()
	if debug {
		fmt.Println("exit monitorList")
	}

}

func setUpIntroducer() {
	// Set up local properties
	self := utils.Member{LOCAL_IP, time.Now().Format(utils.LAYOUT_TIME), utils.GenerateNodeId(), "OK"}
	LOCAL_LIST = make(utils.MembershipList)
	LOCAL_LIST[LOCAL_ID] = self
	LIST_TO_SEND = LOCAL_LIST
	// Set up channels
	HeartbeatChannel = make(chan utils.HeartBeat, 1024)
	StopChannel = make(chan struct{})

	// Start timer and listen to heartbeats
	go introducerTimer(StopChannel)
	go handleHeartBeat(StopChannel)
	go receiveTCP()
}

func introducerTimer(stopCh <-chan struct{}) {
	ticker := time.NewTicker(utils.Period * time.Millisecond)
	defer ticker.Stop()
	var wg sync.WaitGroup
	for {
		select {
		case <-stopCh:
			fmt.Println("Timer stopped")
			return
		case <-ticker.C:

			// Monitor the list
			monitorList()
			wg.Add(1)
			// Send heartbeat in a separate Goroutine
			go sendUDP(&wg)

		}
	}
}

func receiveTCP() {
	tcpAddr, err := net.ResolveTCPAddr("tcp", ":"+utils.INTRODUCER_PORT)
	if err != nil {
		log.Fatal("Error: Incorrect TCP address", err)
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	fmt.Println("Introducer starts listening")
	if err != nil {
		log.Fatal(err)
	}
	for {
		connection, err := listener.Accept()
		if err != nil {
			fmt.Println("Error: connection failed", err)
		}
		go handleConnection(connection)
	}
}

func handleConnection(conn net.Conn) {
	fmt.Println("handle conn")
	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Error reading from TCP connection: ", err)
			continue
		}
		var member utils.Member
		msg := utils.Intro{}
		json.Unmarshal(buffer[:n], &msg)
		member = msg.Mem

		switch msg.Cmd {

		case "JOIN":
			fmt.Println(member.Id + " joined")
			LOCAL_LIST[member.Id] = member
			LIST_TO_SEND = LOCAL_LIST
		case "LEAVE":
			fmt.Println(member.Id + " Left")
			_, ok := LOCAL_LIST[member.Id]
			if ok {
				member.Status = "FAILED"
			}
			LOCAL_LIST[member.Id] = member
			LIST_TO_SEND = LOCAL_LIST
		case "ON":
			SUSPICION = "ON"
			utils.Tclear = 2000
			fmt.Println(member.Id + " enabled suspicion")

		case "OFF":
			SUSPICION = "OFF"
			utils.Tclear = 0
			fmt.Println(member.Id + " disabled suspicion")
		}
	}

}

func sendUDP(group *sync.WaitGroup) {
	defer group.Done()
	for _, member := range LIST_TO_SEND {
		if strings.Compare(member.Id, LOCAL_ID) == 0 {
			continue
		}
		udpAddr := member.IPAddress + ":" + utils.PORT
		conn, err := net.Dial("udp", udpAddr)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer conn.Close()

		hb := utils.HeartBeat{LOCAL_ID, SUSPICION, LIST_TO_SEND}
		message, err := json.Marshal(hb)
		if err != nil {
			fmt.Println("Error marshaling data: ", err)
			return
		}
		conn.Write(message)
	}
}

func main() {
	setUpIntroducer()
	go listenUserInputIntroducer()
	for exit == false {
	}
}
