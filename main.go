package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	_ "image/png"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/pion/randutil"
	"github.com/pion/webrtc/v4"
)

var img *ebiten.Image
const messageSize = 15

func init() {
	var err error
	img, _, err = ebitenutil.NewImageFromFile("gopher.png")
	if err != nil {
		log.Fatal(err)
	}
}

type Player struct{
	positionX int
	positionY int
	speed float64
}

type InputData struct{
	vecX int
	vecY int
}

// implements ebiten.Game interface
type Game struct{
	player Player
	inputData InputData
}

func NewGame() ebiten.Game {
	g := &Game{}
	g.player = Player{positionX: 80, positionY: 80, speed: 5}
	g.inputData = InputData{0, 0}
	return g
}

func (g *Game) updateInputs() error {
	var (
		vec_x = 0.0
		vec_y = 0.0
	)
	// vertical
	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		vec_y = -1
	} else if ebiten.IsKeyPressed(ebiten.KeyDown) {
		vec_y = 1
	}

	// horizontal
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		vec_x = -1
	} else if ebiten.IsKeyPressed(ebiten.KeyRight) {
		vec_x = 1
	}

	// normalize the vector
	if (vec_x != 0 || vec_y != 0) {
		var vector_length = math.Sqrt(vec_x * vec_x + vec_y * vec_y)
		vec_x /= vector_length
		vec_y /= vector_length
		// multiply it by player speed
		vec_x *= g.player.speed
		vec_y *= g.player.speed
	}
	
	// cast input to int for determinism
	g.inputData.vecX = int(vec_x)
	g.inputData.vecY = int(vec_y)

	// if update returns non nil error, game suspends
	return nil
}

// called every tick (default 60 times a second)
// updates game logical state
func (g *Game) Update() error {
	g.updateInputs()

	g.player.positionX += g.inputData.vecX
	g.player.positionY += g.inputData.vecY

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
	op.GeoM.Translate(float64(g.player.positionX), float64(g.player.positionY))
	screen.DrawImage(img, op)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return 640, 480
}

// ReadLoop shows how to read from the datachannel directly
func ReadLoop(d io.Reader) {
	for {
		buffer := make([]byte, messageSize)
		n, err := d.Read(buffer)
		if err != nil {
			fmt.Println("Datachannel closed; Exit the readloop:", err)
			return
		}

		fmt.Printf("Message from DataChannel: %s\n", string(buffer[:n]))
	}
}

// WriteLoop shows how to write to the datachannel directly
func WriteLoop(d io.Writer) {
	for range time.NewTicker(5 * time.Second).C {
		message, err := randutil.GenerateCryptoRandomString(messageSize, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
		if err != nil {
			panic(err)
		}

		fmt.Printf("Sending %s \n", message)
		if _, err := d.Write([]byte(message)); err != nil {
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

// entry point of the program
func main() {
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
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		panic(err)
	}
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("cannot close peerConnection: %v\n", cErr)
		}
	}()

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
	decode(readUntilNewline(), &offer)

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
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

	// Output the answer in base64 so we can paste it in browser
	fmt.Println(encode(peerConnection.LocalDescription()))

	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("Hello, World!")

	// triggers the game loop to actually start up
	// if we run into an error, log what it is
	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
