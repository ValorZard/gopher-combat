$Env:GOOS = 'js'
$Env:GOARCH = 'wasm'
go build -o gopher-combat.wasm valorzard/gopher-combat
Remove-Item Env:GOOS
Remove-Item Env:GOARCH

$goroot = go env GOROOT
cp $goroot\misc\wasm\wasm_exec.js .

# serve the files
python3 -m http.server