package main

import (
	_ "image/png"
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

var img *ebiten.Image

var (
	pos_x = 80.0
	pos_y = 80.0
	vec_x = 0.0
	vec_y = 0.0
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

func (g *Game) updateInputs() error {
	// vertical
	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		vec_y = -1
	} else if ebiten.IsKeyPressed(ebiten.KeyDown) {
		vec_y = 1
	} else {
		vec_y = 0
	}

	// horizontal
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		vec_x = -1
	} else if ebiten.IsKeyPressed(ebiten.KeyRight) {
		vec_x = 1
	} else {
		vec_x = 0
	}

	// normalize the vector
	if (vec_x != 0 || vec_y != 0) {
		var vector_length = math.Sqrt(vec_x * vec_x + vec_y * vec_y)
		vec_x /= vector_length
		vec_y /= vector_length
	}

	// if update returns non nil error, game suspends
	return nil
}

// called every tick (default 60 times a second)
// updates game logical state
func (g *Game) Update() error {
	g.updateInputs()

	pos_x += vec_x
	pos_y += vec_y

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
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return 640, 480
}

// entry point of the program
func main() {
	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("Hello, World!")

	// triggers the game loop to actually start up
	// if we run into an error, log what it is
	if err := ebiten.RunGame(&Game{}); err != nil {
		log.Fatal(err)
	}
}
