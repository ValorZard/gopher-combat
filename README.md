# Gopher Combat

There's no actual combat, but this is a pretty nifty demo on how to use [ebitengine](https://ebitengine.org/) and [pion](https://github.com/pion/webrtc) to pull off a cross platform game!
(UI is done using [ebitenui](https://github.com/ebitenui/ebitenui)
You can have a client running on the browser and one running on a desktop and they can talk to each other, provided they are connected to the same signaling server

Requires [this signaling server](https://github.com/ValorZard/go-signaling-server) to be running

you can run this by going either

``go run .``

or
``.\build_wasm.ps1``

Click "Host Game" to get the lobby id, and then share that with the other clients to get connected

Right now this only supports two clients in the same lobby
