package internal

import (
	"context"
	"github.com/rs/zerolog/log"
	"leader-election/pkg/event"
	"leader-election/pkg/executor"
	"net"
	"net/rpc"
	"strconv"
	"strings"
	"time"
)

type Node struct {
	ID       string
	Addr     string
	Peers    *Peers
	eventBus event.Bus

	servers map[string]Server

	leaderId string

	checkScript   string
	checkInterval time.Duration
	checkRetries  int
	checkTimeout  time.Duration

	runScript  string
	runTimeout time.Duration

	stopScript  string
	stopTimeout time.Duration

	checkCh chan bool
}

func NewNode(nodeID string, servers []Server, checkScript string, checkInterval time.Duration, checkRetries int, checkTimeout time.Duration, runScript string, runTimeout time.Duration, stopScript string, stopTimeout time.Duration) *Node {
	node := &Node{
		ID:       nodeID,
		Peers:    NewPeers(),
		eventBus: event.NewBus(),

		servers: make(map[string]Server),

		checkScript:   checkScript,
		checkInterval: checkInterval,
		checkRetries:  checkRetries,
		checkTimeout:  checkTimeout,
		runScript:     runScript,
		runTimeout:    runTimeout,
		stopScript:    stopScript,
		stopTimeout:   stopTimeout,

		checkCh: make(chan bool),
	}

	for _, v := range servers {
		node.servers[v.Id] = v
	}

	node.Addr = node.servers[node.ID].Ip + ":" + strconv.Itoa(node.servers[node.ID].Port)

	log.Info().Msgf("my addr is %s", node.Addr)

	node.eventBus.Subscribe(event.LeaderElected, node.PingLeaderContinuously)

	go func() {
		for {
			select {
			case c := <-node.checkCh:
				if !c {
					log.Info().Msgf("%s is down", node.ID)
				}
			}
		}
	}()

	return node
}

func (node *Node) NewListener() (net.Listener, error) {
	addr, err := net.Listen("tcp", node.Addr)
	return addr, err
}

func (node *Node) ConnectToPeers() {
	for peerID, peerAddr := range node.servers {
		if node.IsItself(peerID) {
			continue
		}

		rpcClient := node.connect(peerAddr.Ip + ":" + strconv.Itoa(peerAddr.Port))
		pingMessage := Message{FromPeerID: node.ID, Type: PING}
		reply, _ := node.CommunicateWithPeer(rpcClient, pingMessage)

		if reply.IsPongMessage() {
			log.Debug().Msgf("%s got pong message from %s", node.ID, peerID)
			node.Peers.Add(peerID, rpcClient)
		}
	}
}

func (node *Node) connect(peerAddr string) *rpc.Client {
	log.Info().Msgf("Connecting to %s", peerAddr)
retry:
	client, err := rpc.Dial("tcp", peerAddr)
	if err != nil {
		log.Debug().Msgf("Error dialing rpc dial %s", err.Error())
		time.Sleep(50 * time.Millisecond)
		goto retry
	}
	return client
}

func (node *Node) CommunicateWithPeer(RPCClient *rpc.Client, args Message) (Message, error) {
	var reply Message

	err := RPCClient.Call("Node.HandleMessage", args, &reply)
	if err != nil {
		log.Debug().Msgf("Error calling HandleMessage %s", err.Error())
	}

	return reply, err
}

func (node *Node) HandleMessage(args Message, reply *Message) error {
	reply.FromPeerID = node.ID

	switch args.Type {
	case ELECTION:
		reply.Type = ALIVE
	case ELECTED:
		leaderID := args.FromPeerID
		log.Info().Msgf("Election is done. %s has a new leader %s", node.ID, leaderID)
		node.eventBus.Emit(event.LeaderElected, leaderID)
		reply.Type = OK
	case PING:
		reply.Type = PONG
	}

	return nil
}

func (node *Node) Elect() {
	isHighestRankedNodeAvailable := false

	peers := node.Peers.ToList()
	for i := range peers {
		peer := peers[i]

		if node.IsRankHigherThan(peer.ID) {
			continue
		}

		log.Debug().Msgf("%s send ELECTION message to peer %s", node.ID, peer.ID)
		electionMessage := Message{FromPeerID: node.ID, Type: ELECTION}

		reply, _ := node.CommunicateWithPeer(peer.RPCClient, electionMessage)

		if reply.IsAliveMessage() {
			isHighestRankedNodeAvailable = true
		}
	}

	if !isHighestRankedNodeAvailable {
		leaderID := node.ID
		electedMessage := Message{FromPeerID: leaderID, Type: ELECTED}
		node.BroadcastMessage(electedMessage)
		node.leaderId = leaderID
		log.Info().Msgf("%s is a new leader", node.ID)
		node.StartService(context.Background())
		go node.CheckService(context.Background())
	}
}

func (node *Node) BroadcastMessage(args Message) {
	peers := node.Peers.ToList()
	for i := range peers {
		peer := peers[i]
		node.CommunicateWithPeer(peer.RPCClient, args)
	}
}

func (node *Node) PingLeaderContinuously(_ string, payload any) {
	leaderID := payload.(string)

ping:
	leader := node.Peers.Get(leaderID)
	if leader == nil {
		log.Error().Msgf("%s, %s, %s", node.ID, leaderID, node.Peers.ToIDs())
		return
	}

	pingMessage := Message{FromPeerID: node.ID, Type: PING}
	reply, err := node.CommunicateWithPeer(leader.RPCClient, pingMessage)
	if err != nil {
		log.Info().Msgf("Leader is down, new election about to start!")
		node.Peers.Delete(leaderID)
		node.Elect()
		return
	}

	if reply.IsPongMessage() {
		log.Debug().Msgf("Leader %s sent PONG message", reply.FromPeerID)
		time.Sleep(3 * time.Second)
		goto ping
	}
}

func (node *Node) IsRankHigherThan(id string) bool {
	return strings.Compare(node.ID, id) == 1
}

func (node *Node) IsItself(id string) bool {
	return node.ID == id
}

func (node *Node) CheckService(ctx context.Context) {
	var err error
	var cancel context.CancelFunc
	var timeoutCtx context.Context

	for {
		select {
		case <-ctx.Done():
			cancel()
			return
		default:
			if node.leaderId == node.ID {
				timeoutCtx, cancel = context.WithTimeout(ctx, node.checkTimeout)
				err = executor.Execute(timeoutCtx, node.checkScript)
				if err != nil {
					node.checkCh <- false
				} else {
					node.checkCh <- true
				}
			}
		}
		time.Sleep(node.checkInterval)
	}
}

func (node *Node) StartService(ctx context.Context) {
	timeoutCtx, cancel := context.WithTimeout(ctx, node.runTimeout)
	defer cancel()

	log.Info().Msgf("%s is starting service", node.ID)
	err := executor.Execute(timeoutCtx, node.runScript)
	if err != nil {
		node.checkCh <- false
	} else {
		node.checkCh <- true
	}
}

func (node *Node) StopService(ctx context.Context) {
	timeoutCtx, cancel := context.WithTimeout(ctx, node.stopTimeout)
	defer cancel()

	log.Info().Msgf("%s is stopping service", node.ID)
	err := executor.Execute(timeoutCtx, node.stopScript)
	if err != nil {
		node.checkCh <- false
	} else {
		node.checkCh <- true
	}
}
