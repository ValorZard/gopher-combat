$Env:GOOS = 'js'
$Env:GOARCH = 'wasm'
go build -o gopher-combat.wasm valorzard/gopher-combat
Remove-Item Env:GOOS
Remove-Item Env:GOARCH

$goroot = go env GOROOT
# have to copy the wasm_exec.js file to the current directory
# not really sure where it should be, but this is where it is on my computer
cp $goroot\lib\wasm\wasm_exec.js .

# serve the files
python3 -m http.server