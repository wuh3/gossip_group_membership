package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"mp2/utils"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

var exit = false
var debug = false
var LOCAL_IP, _ = os.Hostname()
var LOCAL_ID = utils.GenerateNodeId()
var LOCAL_LIST utils.MembershipList
var LIST_TO_SEND utils.MembershipList
var LOCAL_VERSION = int64(0)
var SUSPICION = "OFF"

var heartbeatChannel chan utils.HeartBeat
var stopChannel chan struct{}
var printFive bool
var printTen bool

func listSelfId() {
	member, ok := LIST_TO_SEND[LOCAL_ID]
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
			fmt.Println()
		}
	}
}

func setUpNode() {
	// Set up local properties
	fmt.Println("Local IP: " + LOCAL_IP)
	self := utils.Member{LOCAL_IP, time.Now().Format(utils.LAYOUT_TIME), utils.GenerateNodeId(), "OK"}
	LOCAL_LIST = make(utils.MembershipList)
	LOCAL_LIST[LOCAL_ID] = self
	LIST_TO_SEND = LOCAL_LIST
	join(self)
	// Set up channels
	heartbeatChannel = make(chan utils.HeartBeat, 1024)
	stopChannel = make(chan struct{})

	printFive = false
	printTen = false

	// Start timer and listen to heartbeats
	go timer(stopChannel)
	go handleHeartBeat(stopChannel)
}

/**
TODO:
	Compare and update the local lost with the received heartbeat
	No deletion performed here
*/

func updateList(list utils.MembershipList) {
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
	if len(LOCAL_LIST) == 5 && printFive == false {
		listMembership()
		printFive = true
	} else if len(LOCAL_LIST) == 10 && printTen == false {
		listMembership()
		printTen = true
	}
}

func deleteFailedNode() {
	for _, member := range LOCAL_LIST {
		if member.Status == "FAILED" && strings.Compare(member.Id, LOCAL_ID) != 0 {
			delete(LOCAL_LIST, member.Id)
		}
	}
}

/*
*
TODO:
 1. Constantly monitor the local membership list, mark suspect and failed
 2. No deletion performed here
*/
func monitorList() {
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
					m.Status = "SUSPECT"
					m.Version = time.Now().Format(utils.LAYOUT_TIME)
					LOCAL_LIST[m.Id] = *m
					fmt.Println("Node " + m.Id + " is suspected!")
					listMembership()
				} else if m.Status == "SUSPECT" && diff > 2000*time.Millisecond && strings.Compare(m.IPAddress, utils.INTRODUCER_IP) != 0 {
					m.Status = "FAILED"
					LOCAL_LIST[m.Id] = *m
				}
			} else if strings.Compare(SUSPICION, "OFF") == 0 {
				if m.Status == "OK" && diff > 4000*time.Millisecond {
					m.Status = "FAILED"
					LOCAL_LIST[m.Id] = *m
					fmt.Println("Node " + m.Id + " is failed!")
					listMembership()
				}
			}
		}(&member, &wg)
	}
	LIST_TO_SEND = LOCAL_LIST
}

/*
*
TODO:
 1. Receive HB in this method
 2. UpdateList()
 3. Pass receiver id to hbChannel as avoidID
*/
func handleHeartBeat(stopChan chan struct{}) {
	receiverAddress := ":" + utils.PORT
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
		select {
		case <-stopChan:
			fmt.Println("Stopping heartbeat handler...")
			conn.Close()
			return
		default:
			n, _, err := conn.ReadFromUDP(buffer)
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
				// Forward the received heartbeat to the heartbeat channel
				heartbeatChannel <- receivedHB

				// Update the local membership list
				receivedList := receivedHB.List
				updateList(receivedList)
			}
		}
	}
}

/*
TODO:
 1. Randomly select 4 nodes from the LOCAL_LIST other than the avoidID
 2. Send heart beat to selected members using UDP
*/
func sendHeartBeat() {

	rand.Seed(time.Now().UnixNano())

	selectedMembers := make([]utils.Member, 0)

	// Shuffle the keys of the map
	keys := make([]string, 0, len(LOCAL_LIST))
	for key := range LOCAL_LIST {
		keys = append(keys, key)
	}
	rand.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })

	// Select four members with unique IDs
	for _, key := range keys {
		if len(selectedMembers) >= len(keys) || len(selectedMembers) == 4 {
			break
		}
		member := LIST_TO_SEND[key]
		selectedMembers = append(selectedMembers, member)
	}
	var wg sync.WaitGroup
	for _, key := range selectedMembers {
		if strings.Compare(key.Id, LOCAL_ID) == 0 {
			continue
		}
		_, ok := LIST_TO_SEND[key.Id]
		if ok {
			//fmt.Println("Sending hb to :", key.Id)
			wg.Add(1)
			go func(key *utils.Member, group *sync.WaitGroup) {
				defer wg.Done()
				receiverAddress := key.Id
				udpAddr, err := net.ResolveUDPAddr("udp", receiverAddress)
				if err != nil {
					fmt.Println("Error resolving address:", err)
					return
				}

				// Create a UDP connection
				conn, err := net.DialUDP("udp", nil, udpAddr)
				if err != nil {
					fmt.Println("Error creating UDP connection:", err)
					return
				}
				defer conn.Close()

				hb := utils.HeartBeat{LOCAL_ID, SUSPICION, LIST_TO_SEND}

				// Marshal the Member to JSON
				message, err := json.Marshal(hb)
				if err != nil {
					fmt.Println("Error marshalling HeartBeat to JSON:", err)
					return
				}

				// Send the JSON-encoded message
				_, err = conn.Write(message)
				if err != nil {
					fmt.Println("Error sending message:", err)
					return
				}
			}(&key, &wg)
		}
	}
}

func timer(stopCh <-chan struct{}) {
	ticker := time.NewTicker(utils.Period * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			fmt.Println("Process stopped")
			return
		case <-ticker.C:

			// Monitor the list
			monitorList()

			// Send heartbeat in a separate Goroutine

			go sendHeartBeat()

			// Delete failed nodes
			deleteFailedNode()
		}
	}
}

func join(m utils.Member) {
	joinMsg := utils.Intro{Cmd: "JOIN", Mem: m}
	jsonData, _ := json.Marshal(joinMsg)
	conn, err := net.Dial("tcp", utils.INTRODUCER_IP+":"+utils.INTRODUCER_PORT)
	if err != nil {
		fmt.Println("Error sending join msg to introducer:", err)
		return
	}
	defer conn.Close()
	_, err = conn.Write(jsonData)
	if err != nil {
		log.Fatal("Join cluster failed for: ", m.Id, err)
	}
}

func leave() {
	leaveMsg := utils.Intro{Cmd: "LEAVE", Mem: utils.Member{}}
	jsonData, _ := json.Marshal(leaveMsg)
	conn, err := net.Dial("tcp", utils.INTRODUCER_IP+":"+utils.INTRODUCER_PORT)
	if err != nil {
		fmt.Println("Error sending leave msg to introducer:", err)
		return
	}
	defer conn.Close()
	_, err = conn.Write(jsonData)
	if err != nil {
		log.Fatal("Leave cluster failed for: ", LOCAL_ID, err)
	}
	close(stopChannel)
	fmt.Println(LOCAL_ID + " has left the cluster")
}

func toggleSuspicion(cmd string, mem utils.Member) {
	msg := utils.Intro{Cmd: cmd, Mem: mem}
	jsonData, _ := json.Marshal(msg)
	conn, err := net.Dial("tcp", utils.INTRODUCER_IP+":"+utils.INTRODUCER_PORT)
	if err != nil {
		fmt.Println("Error dialing to introducer:", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write(jsonData)
	if err != nil {
		log.Fatal("Send msg to introducer failed for: ", LOCAL_ID, err)
	}
	conn.Close()
	if strings.Compare(cmd, "LEAVE") == 0 {
		close(stopChannel)
	}
}

/*
TODO:
 1. Constantly listen to user inputs
*/

func listenUserInput() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Enter commands from this list: join, list_mem, list_self, leave, enable suspicion, disable suspicion")

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

		case "join":
			setUpNode()
		case "list_mem":
			listMembership()
		case "list_self":
			listSelfId()
		case "leave":
			leave()
		case "enable suspicion":
			//SUSPICION = "ON"
			//utils.Tclear = 2000
			toggleSuspicion("ON", utils.Member{Id: LOCAL_ID})
			fmt.Println("Suspicion enabled")
		case "disable suspicion":
			toggleSuspicion("OFF", utils.Member{Id: LOCAL_ID})
			//SUSPICION = "OFF"
			//utils.Tclear = 0
			fmt.Println("Suspicion disabled")
		case "exit":
			exit = true
		default:
			fmt.Printf("Wrong cmd entered: %s\n", userInput)
			fmt.Println("Please input command from this list: join, list_mem, list_self, leave, enable suspicion, disable suspicion")
		}

	}
}

func main() {
	var cmd string = ""
	if len(os.Args) >= 2 {
		cmd = os.Args[1]
	}
	setUpNode()
	if strings.Compare(cmd, "noinput") != 0 {
		go listenUserInput()
	}
	for exit == false {
	}
}
