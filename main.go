package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"

	//"github.com/pion/randutil"
	_ "image/png"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/kelindar/binary"
)

var img *ebiten.Image

var (
	pos_x        = 80.0
	pos_y        = 80.0
	remote_pos_x = 80.0
	remote_pos_y = 80.0
)

func init() {
	var err error
	img, _, err = ebitenutil.NewImageFromFile("gopher.png")
	if err != nil {
		log.Fatal(err)
	}
}

// implements ebiten.Game interface
type Game struct{}

// called every tick (default 60 times a second)
// updates game logical state
func (g *Game) Update() error {

	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		pos_y -= 1
	}

	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		pos_y += 1
	}

	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		pos_x -= 1
	}

	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		pos_x += 1
	}

	// if update returns non nil error, game suspends
	return nil
}

// called every frame, depends on the monitor refresh rate
// which will probably be at least 60 times per second
func (g *Game) Draw(screen *ebiten.Image) {
	// prints something on the screen
	ebitenutil.DebugPrint(screen, "Hello, World!")

	// draw image
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(pos_x, pos_y)
	screen.DrawImage(img, op)

	// draw remote
	op2 := &ebiten.DrawImageOptions{}
	op2.GeoM.Translate(remote_pos_x, remote_pos_y)
	screen.DrawImage(img, op2)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return 640, 480
}

var (
	// probably move all webrtc networking stuff to a struct i can manage
	peerConnection *webrtc.PeerConnection
)

const messageSize = 32

func startConnection(isHost bool) {
	// Since this behavior diverges from the WebRTC API it has to be
	// enabled using a settings engine. Mixing both detached and the
	// OnMessage DataChannel API is not supported.

	// Create a SettingEngine and enable Detach
	s := webrtc.SettingEngine{}
	s.DetachDataChannels()

	// Create an API object with the engine
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))

	// Everything below is the Pion WebRTC API! Thanks for using it ❤️.

	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection using the API object
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		panic(err)
	}

	// Set the global variable to the newly created RTCPeerConnection
	peerConnection = pc

	// Set the handler for Peer connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())

		if s == webrtc.PeerConnectionStateFailed {
			// Wait until PeerConnection has had no network activity for 30 seconds or another failure. It may be reconnected using an ICE Restart.
			// Use webrtc.PeerConnectionStateDisconnected if you are interested in detecting faster timeout.
			// Note that the PeerConnection may come back from PeerConnectionStateDisconnected.
			fmt.Println("Peer Connection has gone to failed exiting")
			os.Exit(0)
		}

		if s == webrtc.PeerConnectionStateClosed {
			// PeerConnection was explicitly closed. This usually happens from a DTLS CloseNotify
			fmt.Println("Peer Connection has gone to closed exiting")
			os.Exit(0)
		}
	})

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})

	// client to the HTTP signaling server
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// the one that gives the answer is the host
	if isHost {
		
		// Host creates lobby
		lobby_resp, err := client.Get("http://localhost:3000/lobby/host")
		if err != nil {
			panic(err)
		}
		bodyBytes, err := io.ReadAll(lobby_resp.Body)
		if err != nil {
			panic(err)
		}
		lobby_id := string(bodyBytes)
		fmt.Printf("Lobby ID: %s\n", lobby_id)
		

		// Register data channel creation handling
		peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
			fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())

			// Register channel opening handling
			d.OnOpen(func() {
				fmt.Printf("Data channel '%s'-'%d' open.\n", d.Label(), d.ID())

				// Detach the data channel
				raw, dErr := d.Detach()
				if dErr != nil {
					panic(dErr)
				}

				// Handle reading from the data channel
				go ReadLoop(raw)

				// Handle writing to the data channel
				go WriteLoop(raw)
			})
		})

		// Wait for the offer to be pasted
		offer := webrtc.SessionDescription{}
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			offer_resp, err := client.Get("http://localhost:3000/offer/get")
			if err != nil {
				panic(err)
			}
			if offer_resp.StatusCode != http.StatusOK {
				continue
			}
			err = json.NewDecoder(offer_resp.Body).Decode(&offer)
			if err != nil {
				panic(err)
			}
			// Set the remote SessionDescription
			err = peerConnection.SetRemoteDescription(offer)
			if err != nil {
				panic(err)
			}
			// if we have successfully set the remote description, we can break out of the loop
			break
		}

		// Create answer
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			panic(err)
		}

		// Create channel that is blocked until ICE Gathering is complete
		gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

		// Sets the LocalDescription, and starts our UDP listeners
		err = peerConnection.SetLocalDescription(answer)
		if err != nil {
			panic(err)
		}

		// Block until ICE Gathering is complete, disabling trickle ICE
		// we do this because we only can exchange one signaling message
		// in a production application you should exchange ICE Candidates via OnICECandidate
		<-gatherComplete

		// send answer we generated to the signaling server
		answerJson, err := json.Marshal(peerConnection.LocalDescription())
		if err != nil {
			panic(err)
		}
		client.Post("http://localhost:3000/answer/post", "application/json", bytes.NewBuffer(answerJson))
	} else {
		// Create a datachannel with label 'data'
		dataChannel, err := peerConnection.CreateDataChannel("data", nil)
		if err != nil {
			panic(err)
		}

		// Register channel opening handling
		dataChannel.OnOpen(func() {
			fmt.Printf("Data channel '%s'-'%d' open.\n", dataChannel.Label(), dataChannel.ID())

			// Detach the data channel
			raw, dErr := dataChannel.Detach()
			if dErr != nil {
				panic(dErr)
			}

			// Handle reading from the data channel
			go ReadLoop(raw)

			// Handle writing to the data channel
			go WriteLoop(raw)
		})

		// Create an offer to send to the browser
		offer, err := peerConnection.CreateOffer(nil)
		if err != nil {
			panic(err)
		}

		// Sets the LocalDescription, and starts our UDP listeners
		err = peerConnection.SetLocalDescription(offer)
		if err != nil {
			panic(err)
		}

		// print out possible offers from different ICE Candidates
		peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			if candidate != nil {
				offerJson, err := json.Marshal(peerConnection.LocalDescription())
				if err != nil {
					panic(err)
				}
				client.Post("http://localhost:3000/offer/post", "application/json", bytes.NewBuffer(offerJson))
			}
		})

		answer := webrtc.SessionDescription{}
		// read answer from other peer (wait till we actually get something)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			answer_resp, err := client.Get("http://localhost:3000/answer/get")
			if err != nil {
				panic(err)
			}
			if answer_resp.StatusCode != http.StatusOK {
				continue
			}
			err = json.NewDecoder(answer_resp.Body).Decode(&answer)
			if err != nil {
				panic(err)
			}

			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				panic(err)
			}

			// if we have successfully set the remote description, we can break out of the loop
			break
		}
	}
}

func closeConnection() {
	if cErr := peerConnection.Close(); cErr != nil {
		fmt.Printf("cannot close peerConnection: %v\n", cErr)
	}
}

// entry point of the program
func main() {

	isHost := false
	if runtime.GOOS != "js" {
		argsWithProg := os.Args
		isHost = len(argsWithProg) > 1 && argsWithProg[1] == "host"
	}
	startConnection(isHost)
	defer closeConnection()

	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("Hello, World!")

	// triggers the game loop to actually start up
	// if we run into an error, log what it is
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}

type Packet struct {
	Pos_x float64
	Pos_y float64
}

// ReadLoop shows how to read from the datachannel directly
func ReadLoop(d io.Reader) {
	for {
		buffer := make([]byte, messageSize)
		_, err := io.ReadFull(d, buffer)
		if err != nil {
			fmt.Println("Datachannel closed; Exit the readloop:", err)
			return
		}

		var packet Packet
		err = binary.Unmarshal(buffer, &packet)
		if err != nil {
			panic(err)
		}

		remote_pos_x = packet.Pos_x
		remote_pos_y = packet.Pos_y

		fmt.Printf("Message from DataChannel: %f %f\n", packet.Pos_x, packet.Pos_y)
	}
}

// WriteLoop shows how to write to the datachannel directly
func WriteLoop(d io.Writer) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		packet := &Packet{pos_x, pos_y}
		fmt.Printf("Sending x:%f y:%f\n", packet.Pos_x, packet.Pos_y)
		encoded, err := binary.Marshal(packet)
		if err != nil {
			panic(err)
		}

		if _, err := d.Write(encoded); err != nil {
			panic(err)
		}
	}
}

// Read from stdin until we get a newline
func readUntilNewline() (in string) {
	var err error

	r := bufio.NewReader(os.Stdin)
	for {
		in, err = r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}

		if in = strings.TrimSpace(in); len(in) > 0 {
			break
		}
	}

	fmt.Println("")
	return
}

// JSON encode + base64 a SessionDescription
func encode(obj *webrtc.SessionDescription) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

// Decode a base64 and unmarshal JSON into a SessionDescription
func decode(in string, obj *webrtc.SessionDescription) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}

	if err = json.Unmarshal(b, obj); err != nil {
		panic(err)
	}
}
