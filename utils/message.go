package utils

type Member struct {
	IPAddress string
	Version   string
	Id        string
	Status    string
}

type MembershipList map[string]Member

type HeartBeat struct {
	Id        string
	Suspicion string
	List      MembershipList
}

type Intro struct {
	Cmd string
	Mem Member
}

const INTRODUCER_PORT = "8888"
const Tfail = 4000 // 1s for Tfail
var Tclear = 0     // 1s for Tclear
var INTRODUCER_IP string = "fa23-cs425-7410.cs.illinois.edu"

const DropRate = 5
const Period = 1000

var LAYOUT_TIME string = "2006-01-02 03:04:05"
